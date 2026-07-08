package smscodec

import (
	"encoding/binary"
	"fmt"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"
)

type TextEncoding string

const (
	TextEncodingGSM7 TextEncoding = "gsm7"
	TextEncoding8Bit TextEncoding = "8bit"
	TextEncodingUCS2 TextEncoding = "ucs2"
)

type DecodedIncomingSMS struct {
	Index     uint32
	Storage   uint8
	Sender    string
	Message   string
	Timestamp time.Time
	Concat    ConcatInfo
	DCS       byte
	Encoding  TextEncoding
	UDHI      bool
	UDHLength int
}

type deliverLayout struct {
	firstOctet byte
	sender     string
	pid        byte
	dcs        byte
	timestamp  time.Time
	udl        int
	udOffset   int
}

func DecodeIncomingPDU(raw []byte, storageType uint8, index uint32) (*DecodedIncomingSMS, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("PDU too short")
	}
	candidates := incomingPDUCandidates(raw)
	if len(candidates) == 0 {
		return nil, fmt.Errorf("PDU decode failed: no supported SMS-DELIVER candidate")
	}

	errs := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		msg, err := DecodeDeliverTPDUDetailed(candidate.tpdu)
		if err == nil {
			msg.Index = index
			msg.Storage = storageType
			return msg, nil
		}
		errs = append(errs, fmt.Sprintf("%s: %v", candidate.name, err))
	}
	return nil, fmt.Errorf("PDU decode failed: %s", strings.Join(errs, "; "))
}

func DecodeDeliverTPDUDetailed(tpduBytes []byte) (*DecodedIncomingSMS, error) {
	if trimmed, ok := TrimDeliverTPDUToDeclaredLength(tpduBytes); ok {
		tpduBytes = trimmed
	}
	if normalized, ok := normalizeDeliverTPDUGSM7SpareBits(tpduBytes); ok {
		tpduBytes = normalized
	}

	layout, err := parseDeliverLayout(tpduBytes)
	if err != nil {
		return nil, err
	}

	encoding, err := ParseDCS(layout.dcs)
	if err != nil {
		return nil, err
	}

	udOctets, err := deliverUserDataOctets(tpduBytes, layout, encoding)
	if err != nil {
		return nil, err
	}

	udh, text, err := decodeUserData(layout, encoding, udOctets)
	if err != nil {
		return nil, err
	}

	return &DecodedIncomingSMS{
		Sender:    layout.sender,
		Message:   text,
		Timestamp: layout.timestamp,
		Concat:    ParseUDH(udh),
		DCS:       layout.dcs,
		Encoding:  encoding,
		UDHI:      layout.firstOctet&0x40 != 0,
		UDHLength: len(udh),
	}, nil
}

func ParseDCS(dcs byte) (TextEncoding, error) {
	if dcs&0xC0 == 0x00 {
		if dcs&0x20 != 0 {
			return "", fmt.Errorf("unsupported compressed SMS DCS 0x%02X", dcs)
		}
		switch (dcs >> 2) & 0x03 {
		case 0:
			return TextEncodingGSM7, nil
		case 1:
			return TextEncoding8Bit, nil
		case 2:
			return TextEncodingUCS2, nil
		default:
			return "", fmt.Errorf("unsupported reserved SMS DCS alphabet 0x%02X", dcs)
		}
	}

	switch dcs & 0xF0 {
	case 0xC0, 0xD0:
		return TextEncodingGSM7, nil
	case 0xE0:
		return TextEncodingUCS2, nil
	case 0xF0:
		if dcs&0x04 != 0 {
			return TextEncoding8Bit, nil
		}
		return TextEncodingGSM7, nil
	default:
		return "", fmt.Errorf("unsupported SMS DCS 0x%02X", dcs)
	}
}

func ParseUDH(udh []byte) ConcatInfo {
	if len(udh) == 0 {
		return ConcatInfo{}
	}
	for i := 0; i+2 <= len(udh); {
		iei := udh[i]
		iel := int(udh[i+1])
		i += 2
		if i+iel > len(udh) {
			return ConcatInfo{}
		}
		data := udh[i : i+iel]
		switch {
		case iei == 0x00 && len(data) >= 3 && int(data[1]) > 1:
			return ConcatInfo{IsConcat: true, Ref: int(data[0]), RefBits: 8, Total: int(data[1]), Seq: int(data[2])}
		case iei == 0x08 && len(data) >= 4 && int(data[2]) > 1:
			return ConcatInfo{IsConcat: true, Ref: int(binary.BigEndian.Uint16(data[:2])), RefBits: 16, Total: int(data[2]), Seq: int(data[3])}
		}
		i += iel
	}
	return ConcatInfo{}
}

func decodeUserData(layout deliverLayout, encoding TextEncoding, ud []byte) ([]byte, string, error) {
	if layout.firstOctet&0x40 == 0 {
		text, err := decodeUserDataText(encoding, ud, layout.udl)
		return nil, text, err
	}
	if len(ud) == 0 {
		return nil, "", fmt.Errorf("UDHI set but user data is empty")
	}
	udhLen := int(ud[0])
	headerBytes := udhLen + 1
	if headerBytes > len(ud) {
		return nil, "", fmt.Errorf("UDH length %d exceeds user data length %d", udhLen, len(ud))
	}
	udh := ud[1:headerBytes]

	if encoding == TextEncodingGSM7 {
		headerSeptets := (headerBytes*8 + 6) / 7
		textSeptets := layout.udl - headerSeptets
		if textSeptets < 0 {
			return nil, "", fmt.Errorf("UDH consumes %d septets but UDL is %d", headerSeptets, layout.udl)
		}
		text, err := decodeGSM7FromBitOffset(ud, headerSeptets*7, textSeptets)
		return udh, text, err
	}

	text, err := decodeUserDataText(encoding, ud[headerBytes:], layout.udl-headerBytes)
	return udh, text, err
}

func decodeUserDataText(encoding TextEncoding, body []byte, septets int) (string, error) {
	switch encoding {
	case TextEncodingGSM7:
		return DecodeGSM7(body, septets)
	case TextEncodingUCS2:
		return DecodeUCS2(body)
	case TextEncoding8Bit:
		if utf8.Valid(body) {
			return string(body), nil
		}
		return fmt.Sprintf("[8-bit SMS data: %d bytes]", len(body)), nil
	default:
		return "", fmt.Errorf("unsupported text encoding %q", encoding)
	}
}

func DecodeUCS2(data []byte) (string, error) {
	if len(data)%2 != 0 {
		return "", fmt.Errorf("invalid UCS-2/UTF-16BE length: %d bytes", len(data))
	}
	if len(data) >= 2 {
		switch {
		case data[0] == 0xFE && data[1] == 0xFF:
			data = data[2:]
		case data[0] == 0xFF && data[1] == 0xFE:
			data = data[2:]
			u16 := make([]uint16, 0, len(data)/2)
			for i := 0; i+1 < len(data); i += 2 {
				u16 = append(u16, binary.LittleEndian.Uint16(data[i:i+2]))
			}
			return string(utf16.Decode(u16)), nil
		}
	}
	u16 := make([]uint16, 0, len(data)/2)
	for i := 0; i+1 < len(data); i += 2 {
		u16 = append(u16, binary.BigEndian.Uint16(data[i:i+2]))
	}
	return string(utf16.Decode(u16)), nil
}

func DecodeGSM7(data []byte, septets int) (string, error) {
	return decodeGSM7FromBitOffset(data, 0, septets)
}

func decodeGSM7FromBitOffset(data []byte, bitOffset int, septets int) (string, error) {
	if septets < 0 {
		return "", fmt.Errorf("invalid GSM7 septet count: %d", septets)
	}
	var b strings.Builder
	escaped := false
	for i := 0; i < septets; i++ {
		v, ok := readSeptet(data, bitOffset+i*7)
		if !ok {
			return "", fmt.Errorf("GSM7 septet %d exceeds %d bytes", i, len(data))
		}
		if escaped {
			if r, ok := gsm7ExtTable[v]; ok {
				b.WriteRune(r)
			}
			escaped = false
			continue
		}
		if v == 0x1B {
			escaped = true
			continue
		}
		b.WriteRune(gsm7DefaultTable[v])
	}
	if escaped {
		return "", fmt.Errorf("dangling GSM7 escape at end of user data")
	}
	return b.String(), nil
}

func readSeptet(data []byte, bitOffset int) (byte, bool) {
	byteOffset := bitOffset / 8
	shift := uint(bitOffset % 8)
	if byteOffset >= len(data) {
		return 0, false
	}
	v := uint16(data[byteOffset]) >> shift
	if shift > 1 {
		if byteOffset+1 >= len(data) {
			return 0, false
		}
		v |= uint16(data[byteOffset+1]) << (8 - shift)
	}
	return byte(v & 0x7F), true
}

func parseDeliverLayout(tpduBytes []byte) (deliverLayout, error) {
	if len(tpduBytes) < 1 || tpduBytes[0]&0x03 != 0 {
		return deliverLayout{}, fmt.Errorf("not an SMS-DELIVER TPDU")
	}
	i := 1
	if i+2 > len(tpduBytes) {
		return deliverLayout{}, fmt.Errorf("TPDU too short for originating address")
	}
	oaLen := int(tpduBytes[i])
	toa := tpduBytes[i+1]
	i += 2
	oaOctets := (oaLen + 1) / 2
	if i+oaOctets > len(tpduBytes) {
		return deliverLayout{}, fmt.Errorf("originating address exceeds TPDU length")
	}
	sender := decodeOriginatingAddress(tpduBytes[i:i+oaOctets], oaLen, toa)
	i += oaOctets

	if i+10 > len(tpduBytes) {
		return deliverLayout{}, fmt.Errorf("TPDU too short for PID/DCS/SCTS/UDL")
	}
	pid := tpduBytes[i]
	dcs := tpduBytes[i+1]
	i += 2
	ts := decodeSCTS(tpduBytes[i : i+7])
	i += 7
	udl := int(tpduBytes[i])
	i++
	return deliverLayout{firstOctet: tpduBytes[0], sender: sender, pid: pid, dcs: dcs, timestamp: ts, udl: udl, udOffset: i}, nil
}

func deliverUserDataOctets(tpduBytes []byte, layout deliverLayout, encoding TextEncoding) ([]byte, error) {
	octets := layout.udl
	if encoding == TextEncodingGSM7 {
		octets = (layout.udl*7 + 7) / 8
	}
	if layout.udOffset+octets > len(tpduBytes) {
		return nil, fmt.Errorf("user data exceeds TPDU length: need %d bytes at offset %d, have %d", octets, layout.udOffset, len(tpduBytes))
	}
	return tpduBytes[layout.udOffset : layout.udOffset+octets], nil
}

type incomingPDUCandidate struct {
	name string
	tpdu []byte
}

func incomingPDUCandidates(raw []byte) []incomingPDUCandidate {
	candidates := make([]incomingPDUCandidate, 0, 3)
	appendCandidate := func(name string, tpduBytes []byte) {
		if len(tpduBytes) == 0 || tpduBytes[0]&0x03 != 0 {
			return
		}
		for _, c := range candidates {
			if string(c.tpdu) == string(tpduBytes) {
				return
			}
		}
		candidates = append(candidates, incomingPDUCandidate{name: name, tpdu: append([]byte(nil), tpduBytes...)})
	}
	if tpdu, ok := extractFullPDUWithSMSC(raw); ok {
		appendCandidate("full_pdu_smsc", tpdu)
	}
	if tpdu, ok := extractIncomingRPDataTPDU(raw); ok {
		appendCandidate("rp_data", tpdu)
	}
	appendCandidate("direct_tpdu", raw)
	return candidates
}

func extractFullPDUWithSMSC(raw []byte) ([]byte, bool) {
	if len(raw) < 2 {
		return nil, false
	}
	smscLen := int(raw[0])
	if smscLen == 0 {
		return raw[1:], true
	}
	if smscLen < 2 || 1+smscLen >= len(raw) {
		return nil, false
	}
	if raw[1]&0x80 == 0 {
		return nil, false
	}
	return raw[1+smscLen:], true
}

func extractIncomingRPDataTPDU(raw []byte) ([]byte, bool) {
	if len(raw) < 5 || raw[0] != 0x01 {
		return nil, false
	}
	i := 2
	if !skipRPAddress(raw, &i) || !skipRPAddress(raw, &i) || i >= len(raw) {
		return nil, false
	}
	udLen := int(raw[i])
	i++
	if udLen <= 0 || i+udLen > len(raw) {
		return nil, false
	}
	return raw[i : i+udLen], true
}

func skipRPAddress(raw []byte, i *int) bool {
	if i == nil || *i >= len(raw) {
		return false
	}
	n := int(raw[*i])
	*i = *i + 1
	if *i+n > len(raw) {
		return false
	}
	*i += n
	return true
}

func decodeOriginatingAddress(data []byte, digits int, toa byte) string {
	ton := (toa >> 4) & 0x07
	if ton == 5 {
		text, err := DecodeGSM7(data, digits)
		if err == nil {
			return text
		}
	}
	return decodeSemiOctetAddress(data, digits, toa)
}

func decodeSemiOctetAddress(data []byte, digits int, toa byte) string {
	var b strings.Builder
	for _, v := range data {
		for _, n := range []byte{v & 0x0F, v >> 4} {
			if b.Len() >= digits {
				break
			}
			if n <= 9 {
				b.WriteByte('0' + n)
			}
		}
	}
	out := b.String()
	if ((toa>>4)&0x07) == 1 && out != "" {
		return "+" + out
	}
	return out
}

func decodeSCTS(data []byte) time.Time {
	if len(data) != 7 {
		return time.Time{}
	}
	decode := func(v byte) int { return int(v&0x0F)*10 + int(v>>4) }
	year := 2000 + decode(data[0])
	month := time.Month(decode(data[1]))
	day := decode(data[2])
	hour := decode(data[3])
	minute := decode(data[4])
	second := decode(data[5])
	if month < 1 || month > 12 || day < 1 || day > 31 || hour > 23 || minute > 59 || second > 59 {
		return time.Time{}
	}
	return time.Date(year, month, day, hour, minute, second, 0, time.UTC)
}

var gsm7DefaultTable = [128]rune{
	'@', '\u00A3', '$', '\u00A5', '\u00E8', '\u00E9', '\u00F9', '\u00EC',
	'\u00F2', '\u00C7', '\n', '\u00D8', '\u00F8', '\r', '\u00C5', '\u00E5',
	'\u0394', '_', '\u03A6', '\u0393', '\u039B', '\u03A9', '\u03A0', '\u03A8',
	'\u03A3', '\u0398', '\u039E', 0, '\u00C6', '\u00E6', '\u00DF', '\u00C9',
	' ', '!', '"', '#', '\u00A4', '%', '&', '\'',
	'(', ')', '*', '+', ',', '-', '.', '/',
	'0', '1', '2', '3', '4', '5', '6', '7',
	'8', '9', ':', ';', '<', '=', '>', '?',
	'\u00A1', 'A', 'B', 'C', 'D', 'E', 'F', 'G',
	'H', 'I', 'J', 'K', 'L', 'M', 'N', 'O',
	'P', 'Q', 'R', 'S', 'T', 'U', 'V', 'W',
	'X', 'Y', 'Z', '\u00C4', '\u00D6', '\u00D1', '\u00DC', '\u00A7',
	'\u00BF', 'a', 'b', 'c', 'd', 'e', 'f', 'g',
	'h', 'i', 'j', 'k', 'l', 'm', 'n', 'o',
	'p', 'q', 'r', 's', 't', 'u', 'v', 'w',
	'x', 'y', 'z', '\u00E4', '\u00F6', '\u00F1', '\u00FC', '\u00E0',
}

var gsm7ExtTable = map[byte]rune{
	0x0A: '\f',
	0x14: '^',
	0x28: '{',
	0x29: '}',
	0x2F: '\\',
	0x3C: '[',
	0x3D: '~',
	0x3E: ']',
	0x40: '|',
	0x65: '\u20AC',
}
