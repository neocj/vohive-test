package smscodec

import (
	"strings"
	"testing"
	"unicode/utf16"
)

func TestDecodeIncomingPDUOfflineSamples(t *testing.T) {
	tests := []struct {
		name string
		tpdu []byte
		want string
	}{
		{name: "gsm7 english", tpdu: buildGSM7DeliverTPDU(t, "10086", "Hello World 123", nil), want: "Hello World 123"},
		{name: "gsm7 extension", tpdu: buildGSM7DeliverTPDU(t, "10086", "^ { } \\ [ ] ~ | \u20AC", nil), want: "^ { } \\ [ ] ~ | \u20AC"},
		{name: "ucs2 chinese", tpdu: buildUCS2DeliverTPDU(t, "10086", "测试中文ABC123结束", nil), want: "测试中文ABC123结束"},
		{name: "ucs2 mixed code", tpdu: buildUCS2DeliverTPDU(t, "10086", "您的验证码是 318357", nil), want: "您的验证码是 318357"},
		{name: "ucs2 template prefix", tpdu: buildUCS2DeliverTPDU(t, "10086", "<#> 您的验证码是 318357", nil), want: "<#> 您的验证码是 318357"},
		{name: "ucs2 japanese", tpdu: buildUCS2DeliverTPDU(t, "10086", "テスト日本語", nil), want: "テスト日本語"},
		{name: "ucs2 korean", tpdu: buildUCS2DeliverTPDU(t, "10086", "테스트한국어", nil), want: "테스트한국어"},
		{name: "ucs2 russian", tpdu: buildUCS2DeliverTPDU(t, "10086", "Привет мир", nil), want: "Привет мир"},
		{name: "ucs2 arabic", tpdu: buildUCS2DeliverTPDU(t, "10086", "مرحبا بالعالم", nil), want: "مرحبا بالعالم"},
		{name: "ucs2 emoji surrogate pair", tpdu: buildUCS2DeliverTPDU(t, "10086", "验证码🙂123", nil), want: "验证码🙂123"},
		{name: "gsm7 with udh", tpdu: buildGSM7DeliverTPDU(t, "10086", "Hello UDH", []byte{0x05, 0x04, 0x00, 0x00, 0x00, 0x00}), want: "Hello UDH"},
		{name: "numeric otp regression", tpdu: buildGSM7DeliverTPDU(t, "10086", "318357", nil), want: "318357"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := DecodeIncomingPDU(fullPDU(tt.tpdu), 1, 7)
			if err != nil {
				t.Fatalf("DecodeIncomingPDU() error = %v", err)
			}
			if msg.Message != tt.want {
				t.Fatalf("message = %q, want %q", msg.Message, tt.want)
			}
		})
	}
}

func TestDecodeIncomingPDURegressionChineseIsNotASCIIFiltered(t *testing.T) {
	msg, err := DecodeIncomingPDU(fullPDU(buildUCS2DeliverTPDU(t, "10086", "<#> 您的验证码是 318357", nil)), 1, 8)
	if err != nil {
		t.Fatalf("DecodeIncomingPDU() error = %v", err)
	}
	if msg.Message != "<#> 您的验证码是 318357" {
		t.Fatalf("message = %q, want full Chinese body", msg.Message)
	}
	if msg.Message == "<#>318357" || !strings.Contains(msg.Message, "您的验证码是") {
		t.Fatalf("message lost non-ASCII content: %q", msg.Message)
	}
}

func TestDecodeIncomingPDU8BitUTF8Data(t *testing.T) {
	tpdu := buildDeliverTPDU(t, "10086", 0x04, false, []byte("binary-ish utf8 text"), len("binary-ish utf8 text"))
	msg, err := DecodeIncomingPDU(fullPDU(tpdu), 1, 9)
	if err != nil {
		t.Fatalf("DecodeIncomingPDU() error = %v", err)
	}
	if msg.Message != "binary-ish utf8 text" {
		t.Fatalf("message = %q", msg.Message)
	}
}

func TestDecodeIncomingPDUUCS2BOM(t *testing.T) {
	body := append([]byte{0xFE, 0xFF}, utf16BE("带BOM中文")...)
	tpdu := buildDeliverTPDU(t, "10086", 0x08, false, body, len(body))
	msg, err := DecodeIncomingPDU(fullPDU(tpdu), 1, 10)
	if err != nil {
		t.Fatalf("DecodeIncomingPDU() error = %v", err)
	}
	if msg.Message != "带BOM中文" {
		t.Fatalf("message = %q", msg.Message)
	}
}

func TestDecodeIncomingPDUMultipart8BitReferenceUCS2(t *testing.T) {
	ref := byte(0x7A)
	part1 := buildUCS2DeliverTPDU(t, "10086", "第一段中文", concat8UDH(ref, 2, 1))
	part2 := buildUCS2DeliverTPDU(t, "10086", "第二段结束", concat8UDH(ref, 2, 2))

	msg1 := mustDecodeIncoming(t, part1)
	msg2 := mustDecodeIncoming(t, part2)
	if !msg1.Concat.IsConcat || msg1.Concat.RefBits != 8 || msg1.Concat.Seq != 1 {
		t.Fatalf("unexpected concat info: %+v", msg1.Concat)
	}

	r := NewReassembler()
	if complete, full := r.AddForDevice("wwan0", msg1.Sender, msg1.Concat, msg1.Message); complete || full != "" {
		t.Fatalf("first fragment = (%v, %q), want incomplete", complete, full)
	}
	complete, full := r.AddForDevice("wwan0", msg2.Sender, msg2.Concat, msg2.Message)
	if !complete || full != "第一段中文第二段结束" {
		t.Fatalf("reassembled = (%v, %q)", complete, full)
	}
}

func TestDecodeIncomingPDUMultipart16BitReferenceUCS2(t *testing.T) {
	ref := uint16(0x1234)
	part1 := buildUCS2DeliverTPDU(t, "10086", "甲乙丙", concat16UDH(ref, 2, 1))
	part2 := buildUCS2DeliverTPDU(t, "10086", "丁戊己", concat16UDH(ref, 2, 2))

	msg1 := mustDecodeIncoming(t, part1)
	msg2 := mustDecodeIncoming(t, part2)
	if !msg1.Concat.IsConcat || msg1.Concat.RefBits != 16 || msg1.Concat.Ref != int(ref) {
		t.Fatalf("unexpected concat info: %+v", msg1.Concat)
	}

	r := NewReassembler()
	r.AddForDevice("wwan0", msg1.Sender, msg1.Concat, msg1.Message)
	complete, full := r.AddForDevice("wwan0", msg2.Sender, msg2.Concat, msg2.Message)
	if !complete || full != "甲乙丙丁戊己" {
		t.Fatalf("reassembled = (%v, %q)", complete, full)
	}
}

func TestDecodeIncomingPDUMultipartOutOfOrderDuplicateAndMissing(t *testing.T) {
	ref := byte(0x42)
	part1 := mustDecodeIncoming(t, buildUCS2DeliverTPDU(t, "10086", "前半", concat8UDH(ref, 2, 1)))
	part2 := mustDecodeIncoming(t, buildUCS2DeliverTPDU(t, "10086", "后半", concat8UDH(ref, 2, 2)))

	r := NewReassembler()
	if complete, _ := r.AddForDevice("wwan0", part2.Sender, part2.Concat, part2.Message); complete {
		t.Fatal("out-of-order first part should not complete")
	}
	if complete, _ := r.AddForDevice("wwan0", part2.Sender, part2.Concat, part2.Message); complete {
		t.Fatal("duplicate fragment should not complete")
	}
	complete, full := r.AddForDevice("wwan0", part1.Sender, part1.Concat, part1.Message)
	if !complete || full != "前半后半" {
		t.Fatalf("out-of-order reassembled = (%v, %q)", complete, full)
	}

	missing := NewReassembler()
	if complete, full := missing.AddForDevice("wwan0", part1.Sender, part1.Concat, part1.Message); complete || full != "" {
		t.Fatalf("missing fragment = (%v, %q), want incomplete", complete, full)
	}
}

func TestDecodeIncomingPDURejectsOddLengthUCS2(t *testing.T) {
	tpdu := buildDeliverTPDU(t, "10086", 0x08, false, []byte{0x4F}, 1)
	_, err := DecodeIncomingPDU(fullPDU(tpdu), 1, 11)
	if err == nil || !strings.Contains(err.Error(), "invalid UCS-2") {
		t.Fatalf("error = %v, want invalid UCS-2 diagnostic", err)
	}
}

func TestDecodeIncomingPDURejectsUnknownDCS(t *testing.T) {
	tpdu := buildDeliverTPDU(t, "10086", 0x0C, false, []byte{0x00}, 1)
	_, err := DecodeIncomingPDU(fullPDU(tpdu), 1, 12)
	if err == nil || !strings.Contains(err.Error(), "reserved SMS DCS") {
		t.Fatalf("error = %v, want reserved DCS diagnostic", err)
	}
}

func mustDecodeIncoming(t *testing.T, tpdu []byte) *DecodedIncomingSMS {
	t.Helper()
	msg, err := DecodeIncomingPDU(fullPDU(tpdu), 1, 1)
	if err != nil {
		t.Fatalf("DecodeIncomingPDU() error = %v", err)
	}
	return msg
}

func fullPDU(tpdu []byte) []byte {
	out := make([]byte, 0, len(tpdu)+1)
	out = append(out, 0x00)
	out = append(out, tpdu...)
	return out
}

func buildGSM7DeliverTPDU(t *testing.T, sender, text string, udhIEs []byte) []byte {
	t.Helper()
	septets := gsm7Septets(t, text)
	udhi := len(udhIEs) > 0
	if !udhi {
		ud := make([]byte, (len(septets)*7+7)/8)
		packSeptetsAt(septets, 0, ud)
		return buildDeliverTPDU(t, sender, 0x00, false, ud, len(septets))
	}

	udh := append([]byte{byte(len(udhIEs))}, udhIEs...)
	headerSeptets := (len(udh)*8 + 6) / 7
	udl := headerSeptets + len(septets)
	ud := make([]byte, (udl*7+7)/8)
	copy(ud, udh)
	packSeptetsAt(septets, headerSeptets*7, ud)
	return buildDeliverTPDU(t, sender, 0x00, true, ud, udl)
}

func buildUCS2DeliverTPDU(t *testing.T, sender, text string, udhIEs []byte) []byte {
	t.Helper()
	body := utf16BE(text)
	udhi := len(udhIEs) > 0
	if udhi {
		udh := append([]byte{byte(len(udhIEs))}, udhIEs...)
		body = append(udh, body...)
	}
	return buildDeliverTPDU(t, sender, 0x08, udhi, body, len(body))
}

func buildDeliverTPDU(t *testing.T, sender string, dcs byte, udhi bool, userData []byte, udl int) []byte {
	t.Helper()
	toa, addr := encodeNumericAddress(t, sender)
	first := byte(0x00)
	if udhi {
		first |= 0x40
	}
	pdu := []byte{first, byte(len(strings.TrimPrefix(sender, "+"))), toa}
	pdu = append(pdu, addr...)
	pdu = append(pdu, 0x00, dcs)
	pdu = append(pdu, []byte{0x62, 0x70, 0x80, 0x21, 0x43, 0x65, 0x00}...)
	pdu = append(pdu, byte(udl))
	pdu = append(pdu, userData...)
	return pdu
}

func encodeNumericAddress(t *testing.T, number string) (byte, []byte) {
	t.Helper()
	toa := byte(0x81)
	digits := number
	if strings.HasPrefix(digits, "+") {
		toa = 0x91
		digits = strings.TrimPrefix(digits, "+")
	}
	for _, r := range digits {
		if r < '0' || r > '9' {
			t.Fatalf("test helper only supports numeric senders: %q", number)
		}
	}
	out := make([]byte, 0, (len(digits)+1)/2)
	for i := 0; i < len(digits); i += 2 {
		lo := digits[i] - '0'
		hi := byte(0x0F)
		if i+1 < len(digits) {
			hi = digits[i+1] - '0'
		}
		out = append(out, lo|(hi<<4))
	}
	return toa, out
}

func utf16BE(s string) []byte {
	u16 := utf16.Encode([]rune(s))
	out := make([]byte, 0, len(u16)*2)
	for _, v := range u16 {
		out = append(out, byte(v>>8), byte(v))
	}
	return out
}

func gsm7Septets(t *testing.T, s string) []byte {
	t.Helper()
	defaultReverse, extReverse := gsm7ReverseTables()
	out := make([]byte, 0, len(s))
	for _, r := range s {
		if v, ok := defaultReverse[r]; ok {
			out = append(out, v)
			continue
		}
		if v, ok := extReverse[r]; ok {
			out = append(out, 0x1B, v)
			continue
		}
		t.Fatalf("character %q is not in GSM 03.38 test table", r)
	}
	return out
}

func gsm7ReverseTables() (map[rune]byte, map[rune]byte) {
	defaultReverse := make(map[rune]byte)
	for i, r := range gsm7DefaultTable {
		if r == 0 || byte(i) == 0x1B {
			continue
		}
		defaultReverse[r] = byte(i)
	}
	extReverse := make(map[rune]byte)
	for k, r := range gsm7ExtTable {
		extReverse[r] = k
	}
	return defaultReverse, extReverse
}

func packSeptetsAt(septets []byte, bitOffset int, out []byte) {
	for i, septet := range septets {
		pos := bitOffset + i*7
		byteOffset := pos / 8
		shift := uint(pos % 8)
		v := uint16(septet&0x7F) << shift
		out[byteOffset] |= byte(v)
		if shift > 1 && byteOffset+1 < len(out) {
			out[byteOffset+1] |= byte(v >> 8)
		}
	}
}

func concat8UDH(ref byte, total, seq byte) []byte {
	return []byte{0x00, 0x03, ref, total, seq}
}

func concat16UDH(ref uint16, total, seq byte) []byte {
	return []byte{0x08, 0x04, byte(ref >> 8), byte(ref), total, seq}
}
