package smscodec

import (
	"errors"
	"fmt"
	"strings"
	"time"

	smspdu "github.com/warthog618/sms"
	"github.com/warthog618/sms/encoding/tpdu"
	"github.com/warthog618/sms/encoding/ucs2"
)

type SMSEncoding string

const (
	SMSEncodingAuto SMSEncoding = "auto"
	SMSEncodingUCS2 SMSEncoding = "ucs2"
)

type SubmitOptions struct {
	Encoding SMSEncoding
}

func NormalizeSMSEncoding(raw string) (SMSEncoding, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", string(SMSEncodingAuto):
		return SMSEncodingAuto, nil
	case string(SMSEncodingUCS2):
		return SMSEncodingUCS2, nil
	default:
		return "", fmt.Errorf("unsupported SMS encoding: %s", raw)
	}
}

func IsHexString(s string) bool {
	if len(s) < 2 || len(s)%2 != 0 {
		return false
	}
	for _, c := range s {
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') {
			continue
		}
		return false
	}
	return true
}

type ConcatInfo struct {
	IsConcat bool
	Ref      int
	RefBits  int
	Total    int
	Seq      int
}

func DecodeDeliverTPDU(tpduBytes []byte) (sender string, text string, ts time.Time, concat ConcatInfo, err error) {
	msg, err := DecodeDeliverTPDUDetailed(tpduBytes)
	if err != nil {
		return "", "", time.Time{}, ConcatInfo{}, err
	}
	return msg.Sender, msg.Message, msg.Timestamp, msg.Concat, nil
}

func IsShortCode(phone string) bool {
	if strings.HasPrefix(phone, "+") {
		return false
	}
	digits := strings.TrimLeft(phone, "0123456789")
	return digits == "" && len(phone) <= 6
}

func BuildSubmitTPDUsWithOptions(to, text string, opts SubmitOptions) ([][]byte, []int, error) {
	normalizedTo := strings.TrimSpace(to)
	encoding, err := NormalizeSMSEncoding(string(opts.Encoding))
	if err != nil {
		return nil, nil, err
	}

	msg := []byte(text)
	encoderOptions := []smspdu.EncoderOption{smspdu.To(normalizedTo)}
	if encoding == SMSEncodingUCS2 {
		msg = ucs2.Encode([]rune(text))
		encoderOptions = append(encoderOptions, smspdu.AsUCS2)
	}

	tpdus, err := smspdu.Encode(msg, encoderOptions...)
	if err != nil {
		return nil, nil, err
	}
	if len(tpdus) == 0 {
		return nil, nil, errors.New("empty TPDU encode result")
	}

	var bytesList [][]byte
	var lenList []int

	for _, pdu := range tpdus {
		if IsShortCode(normalizedTo) {
			da := pdu.DA
			da.SetTypeOfNumber(tpdu.TonUnknown)
			da.SetNumberingPlan(tpdu.NpISDN)
			pdu.DA = da
		}

		b, err := pdu.MarshalBinary()
		if err != nil {
			return nil, nil, err
		}
		bytesList = append(bytesList, b)
		lenList = append(lenList, len(b))
	}

	return bytesList, lenList, nil
}
