package minxdr

import (
	"errors"
	"reflect"
	"time"
)

var customPairs map[string]EncDecPair

//RegisterRType Registers an already existing type to encode / decode
func RegisterRType(typeName string, v EncDecPair) {
	customPairs[typeName] = v
}

func init() {
	customPairs = make(map[string]EncDecPair)
	RegisterRType("time.Time", &timeEncDec{})
	RegisterRType("bytes.Buffer", &byteBufEncDec{})
}

//Default custom types
//time.Time
//bytes.Buffer

//timeEncDec implements the time.Time encoding and decoding as a XDR string with RFC3339 nanosecond encoding
type timeEncDec struct {
}

func (d *timeEncDec) Encode(s *Encoder, v reflect.Value) (int, error) {
	viface := v.Interface()
	if tv, ok := viface.(time.Time); ok {
		return s.EncodeString(tv.Format(time.RFC3339Nano))
	}
	return 0, errors.New("unable to enocde time.Time")
}
func (d *timeEncDec) Decode(s *Decoder, v reflect.Value) (int, error) {
	ts, n, err := s.DecodeString()
	if err != nil {
		return n, err
	}
	ttv, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return n, err
	}
	v.Set(reflect.ValueOf(ttv))
	return n, nil
}

//byteBufEncDec Encodes and Decodes a bytes.Buffer as a flat variable length opaque value
type byteBufEncDec struct {
}

func (d *byteBufEncDec) Encode(s *Encoder, v reflect.Value) (int, error) {
	off := v.FieldByName("off").Int()
	buf := v.FieldByName("buf").Bytes()
	return s.EncodeOpaque(buf[off:])
}

func (d *byteBufEncDec) Decode(s *Decoder, v reflect.Value) (int, error) {
	buf, leng, err := s.DecodeOpaque()
	if err != nil {
		return leng, err
	}
	bf := v.FieldByName("buf")
	bf.Set(reflect.MakeSlice(bf.Type(), len(buf), len(buf)))
	bf.SetLen(len(buf))
	bf.SetBytes(buf)
	v.FieldByName("off").SetInt(0)
	v.FieldByName("lastRead").SetInt(0)
	return leng, nil
}
