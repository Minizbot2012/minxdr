package minxdr

import (
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"time"
)

func Marshal(w io.Writer, v interface{}) (int, error) {
	return NewEncoder(w).Encode(v)
}

type Encoder struct {
	w io.Writer
}

func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{w: w}
}

func (s *Encoder) Encode(v interface{}) (int, error) {
	if v == nil {
		return 0, errors.New("Can't marshal nil interface")
	}
	vf := reflect.ValueOf(v)
	for vf.Kind() == reflect.Ptr {
		if vf.IsNil() {
			return 0, errors.New(fmt.Sprintf("Can't marshal nil pointer %s", vf.Kind().String()))
		}
		vf = vf.Elem()
	}
	return s.encode(vf)
}

func (s *Encoder) indirect(v reflect.Value) reflect.Value {
	rv := v
	for rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	return rv
}

func (s *Encoder) EncodeBool(v bool) (int, error) {
	i := int32(0)
	if v == true {
		i = 1
	}
	return s.EncodeInt(i)
}

func (s *Encoder) EncodeFloat(v float32) (int, error) {
	return s.EncodeUint(math.Float32bits(v))
}

func (s *Encoder) EncodeDouble(v float64) (int, error) {
	return s.EncodeUhyper(math.Float64bits(v))
}

func (s *Encoder) EncodeUint(v uint32) (int, error) {
	var b [4]byte
	b[0] = byte(v >> 24)
	b[1] = byte(v >> 16)
	b[2] = byte(v >> 8)
	b[3] = byte(v)
	n, err := s.w.Write(b[:])
	if err != nil {
		return n, err
	}
	return n, nil

}

func (s *Encoder) EncodeInt(v int32) (int, error) {
	return s.EncodeUint(uint32(v))
}

func (s *Encoder) EncodeUhyper(v uint64) (int, error) {
	var b [8]byte
	b[0] = byte(v >> 56)
	b[1] = byte(v >> 48)
	b[2] = byte(v >> 40)
	b[3] = byte(v >> 32)
	b[4] = byte(v >> 24)
	b[5] = byte(v >> 16)
	b[6] = byte(v >> 8)
	b[7] = byte(v)
	return s.w.Write(b[:])
}

func (s *Encoder) EncodeHyper(v int64) (int, error) {
	return s.EncodeUhyper(uint64(v))
}

func (s *Encoder) EncodeOpaque(v []byte) (int, error) {
	bw, err := s.EncodeUint(uint32(len(v)))
	if err != nil {
		return bw, err
	}
	bw2, err := s.EncodeFixedOpaque(v)
	return bw + bw2, err
}

func (s *Encoder) EncodeFixedOpaque(v []byte) (int, error) {
	l := len(v)
	pad := (4 - (l % 4)) % 4
	bw, err := s.w.Write(v)
	if err != nil {
		return bw, err
	}
	if pad > 0 {
		b := make([]byte, pad)
		pw, err := s.w.Write(b)
		bw += pw
		if err != nil {
			return bw, err
		}
	}
	return bw, nil
}

func (s *Encoder) EncodeString(v string) (int, error) {
	bw, err := s.EncodeUint(uint32(len(v)))
	if err != nil {
		return bw, err
	}
	bw2, err := s.EncodeFixedOpaque([]byte(v))
	return bw + bw2, err
}

func (s *Encoder) encodeFixedArray(v reflect.Value) (int, error) {
	if v.Type().Elem().Kind() == reflect.Uint8 {
		if v.CanAddr() {
			return s.EncodeFixedOpaque(v.Slice(0, v.Len()).Bytes())
		}
		slice := make([]byte, v.Len(), v.Len())
		reflect.Copy(reflect.ValueOf(slice), v)
		return s.EncodeFixedOpaque(slice)
	}
	var bw int
	for i := 0; i < v.Len(); i++ {
		bwi, err := s.encode(v.Index(i))
		bw += bwi
		if err != nil {
			return bw, err
		}
	}
	return bw, nil
}

func (s *Encoder) encodeArray(v reflect.Value) (int, error) {
	numItems := uint32(v.Len())
	bw, err := s.EncodeUint(numItems)
	if err != nil {
		return bw, err
	}
	bw2, err := s.encodeFixedArray(v)
	bw += bw2
	return bw, err
}

func (s *Encoder) encodeMap(v reflect.Value) (int, error) {
	bw, err := s.EncodeUint(uint32(v.Len()))
	if err != nil {
		return bw, err
	}
	for _, key := range v.MapKeys() {
		bwi, err := s.encode(key)
		bw += bwi
		if err != nil {
			return bw, err
		}
		bwi, err = s.encode(v.MapIndex(key))
		bw += bwi
		if err != nil {
			return bw, err
		}
	}
	return bw, nil
}

func (s *Encoder) encodeStruct(v reflect.Value) (int, error) {
	var bw int
	vt := v.Type()
	for i := 0; i < v.NumField(); i++ {
		tf := vt.Field(i)
		if tf.PkgPath != "" {
			continue
		}
		vf := v.Field(i)
		vf = s.indirect(vf)
		bwi, err := s.encode(vf)
		bw += bwi
		if err != nil {
			return bw, err
		}

	}
	return bw, nil
}

func (s *Encoder) encodeInterface(v reflect.Value) (int, error) {
	if v.IsNil() || !v.CanInterface() {
		return 0, errors.New("Cannot encode nil interface")
	}

	// Extract underlying value from the interface and indirect through pointers.
	ve := reflect.ValueOf(v.Interface())
	ve = s.indirect(ve)
	return s.encode(ve)
}

func (s *Encoder) encode(v reflect.Value) (int, error) {
	if !v.IsValid() {
		return 0, errors.New(fmt.Sprintf("Type %s is invalid", v.Kind().String()))
	}
	ve := s.indirect(v)
	if ve.Type().String() == "time.Time" && ve.CanInterface() {
		viface := ve.Interface()
		if tv, ok := viface.(time.Time); ok {
			return s.EncodeString(tv.Format(time.RFC3339Nano))
		}
	}
	switch ve.Kind() {
	case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int:
		return s.EncodeInt(int32(ve.Int()))
	case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint:
		return s.EncodeUint(uint32(ve.Uint()))
	case reflect.Int64:
		return s.EncodeHyper(ve.Int())
	case reflect.Uint64:
		return s.EncodeUhyper(ve.Uint())
	case reflect.Bool:
		return s.EncodeBool(ve.Bool())
	case reflect.Float32:
		return s.EncodeFloat(float32(ve.Float()))
	case reflect.Float64:
		return s.EncodeDouble(ve.Float())
	case reflect.String:
		return s.EncodeString(ve.String())
	case reflect.Array:
		return s.encodeFixedArray(ve)
	case reflect.Slice:
		return s.encodeArray(ve)
	case reflect.Struct:
		return s.encodeStruct(ve)
	case reflect.Map:
		return s.encodeMap(ve)
	case reflect.Interface:
		return s.encodeInterface(ve)
	}
	return 0, errors.New("Go type is unsupported")
}
