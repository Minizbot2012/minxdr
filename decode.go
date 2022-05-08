package minxdr

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"reflect"
)

func Unmarshal(r io.Reader, v interface{}) (int, error) {
	return NewDecoder(r).Decode(v)
}

type Decoder struct {
	r io.Reader
}

func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{r: r}
}

func (s *Decoder) Decode(v interface{}) (int, error) {
	if v == nil {
		return 0, fmt.Errorf("can't unmarshal to nil")
	}
	val := reflect.ValueOf(v)
	if val.Kind() != reflect.Ptr {
		return 0, fmt.Errorf("can't unmarshal to non-pointer value")
	}
	if val.IsNil() && !val.CanSet() {
		return 0, fmt.Errorf("can't unmarshal to unsettable")
	}
	return s.decode(val)
}

func (s *Decoder) indirect(v reflect.Value) (reflect.Value, error) {
	iv := v
	for iv.Kind() == reflect.Ptr {
		isNil := iv.IsNil()
		if isNil && !iv.CanSet() {
			return iv, fmt.Errorf("cannot allocate pointer for type %s", iv.Kind().String())
		}
		if isNil {
			iv.Set(reflect.New(iv.Type().Elem()))
		}
		iv = iv.Elem()
	}
	return iv, nil
}

func (s *Decoder) DecodeBool() (bool, int, error) {
	v, n, err := s.DecodeInt()
	if err != nil {
		return false, n, err
	}
	switch v {
	case 0:
		return false, n, nil
	case 1:
		return true, n, nil
	default:
		return false, n, fmt.Errorf("invalid Boolean Value")
	}
}

func (s *Decoder) DecodeFloat() (float32, int, error) {
	b := make([]byte, 4)
	len, err := s.r.Read(b)
	if err != nil {
		return 0.0, len, err
	}
	val := binary.BigEndian.Uint32(b)
	return math.Float32frombits(val), len, nil
}

func (s *Decoder) DecodeDouble() (float64, int, error) {
	b := make([]byte, 8)
	len, err := s.r.Read(b)
	if err != nil {
		return 0.0, len, err
	}
	val := binary.BigEndian.Uint64(b)
	return math.Float64frombits(val), len, nil
}

func (s *Decoder) DecodeUint() (uint32, int, error) {
	b := make([]byte, 4)
	v, err := s.r.Read(b)
	if err != nil {
		return 0, v, err
	}
	val := binary.BigEndian.Uint32(b)
	return val, v, nil
}

func (s *Decoder) DecodeInt() (int32, int, error) {
	dat, len, err := s.DecodeUint()
	if err != nil {
		return 0, len, err
	}
	return int32(dat), len, err
}

func (s *Decoder) DecodeUhyper() (uint64, int, error) {
	b := make([]byte, 8)
	bl, err := s.r.Read(b)
	if err != nil {
		return 0, bl, err
	}
	val := binary.BigEndian.Uint64(b)
	return val, bl, nil
}

func (s *Decoder) DecodeHyper() (int64, int, error) {
	val, bl, err := s.DecodeUhyper()
	if err != nil {
		return 0, bl, err
	}
	return int64(val), bl, nil
}

func (s *Decoder) DecodeOpaque() ([]byte, int, error) {
	len, br1, err := s.DecodeUint()
	if err != nil {
		return []byte{}, br1, err
	}
	data, br2, err := s.DecodeFixedOpaque(int32(len))
	return data, br1 + br2, err
}

func (s *Decoder) DecodeFixedOpaque(len int32) ([]byte, int, error) {
	pad := (4 - (len % 4)) % 4
	paddedSize := len + pad
	b := make([]byte, int(paddedSize))
	br, err := s.r.Read(b)
	if err != nil {
		return []byte{}, br, err
	}
	return b[:len], br, err
}

func (s *Decoder) DecodeString() (string, int, error) {
	dataLen, br1, err := s.DecodeUint()
	if err != nil {
		return "", br1, err
	}
	if uint(dataLen) > uint(math.MaxInt32) {
		return "", br1, fmt.Errorf("max slice exceded")
	}
	data, br2, err := s.DecodeFixedOpaque(int32(dataLen))
	if err != nil {
		return "", br1 + br2, err
	}
	return string(data), br1 + br2, nil
}

func (s *Decoder) decodeFixedArray(v reflect.Value) (int, error) {
	if v.Type().Elem().Kind() == reflect.Uint8 {
		data, br, err := s.DecodeFixedOpaque(int32(v.Len()))
		if err != nil {
			return br, err
		}
		reflect.Copy(v, reflect.ValueOf(data))
		return br, nil
	}
	var br int
	for i := 0; i < v.Len(); i++ {
		bri, err := s.decode(v.Index(i))
		br += bri
		if err != nil {
			return br, err
		}
	}
	return br, nil
}

func (s *Decoder) decodeArray(v reflect.Value) (int, error) {
	len, br1, err := s.DecodeUint()
	if err != nil {
		return br1, err
	}
	v.Set(reflect.MakeSlice(v.Type(), int(len), int(len)))
	v.SetLen(int(len))
	if v.Type().Elem().Kind() == reflect.Uint8 {
		data, br2, err := s.DecodeFixedOpaque(int32(len))
		if err != nil {
			return br1 + br2, err
		}
		v.SetBytes(data)
		return br1 + br2, nil
	}
	var br int
	for i := 0; i < v.Len(); i++ {
		bri, err := s.decode(v.Index(i))
		br += bri
		if err != nil {
			return br, err
		}
	}
	return br, nil
}

func (s *Decoder) decodeMap(v reflect.Value) (int, error) {
	len, br, err := s.DecodeUint()
	if err != nil {
		return br, err
	}
	mat := v.Type()
	v.Set(reflect.MakeMap(mat))
	kt := mat.Key()
	et := mat.Elem()
	for i := uint32(0); i < len; i++ {
		key := reflect.New(kt).Elem()
		bri, err := s.decode(key)
		br += bri
		if err != nil {
			return br, err
		}
		val := reflect.New(et).Elem()
		bri, err = s.decode(val)
		br += bri
		if err != nil {
			return br, err
		}
		v.SetMapIndex(key, val)
	}
	return br, nil
}

func (s *Decoder) decodeStruct(v reflect.Value) (int, error) {
	var br int
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		tf := t.Field(i)
		if tf.PkgPath != "" {
			continue
		}
		vf := v.Field(i)
		vf, err := s.indirect(vf)
		if err != nil {
			return br, err
		}
		if !vf.CanSet() {
			return br, fmt.Errorf("cannot decode to unsettable %s", vf.Type().String())
		}
		bri, err := s.decode(vf)
		br += bri
		if err != nil {
			return br, err
		}
	}
	return br, nil
}

func (s *Decoder) decodeInterface(v reflect.Value) (int, error) {
	if v.IsNil() && !v.CanInterface() {
		return 0, fmt.Errorf("cannot decode to nil interface")
	}
	ve := reflect.ValueOf(v.Interface())
	ve, err := s.indirect(ve)
	if err != nil {
		return 0, err
	}
	if !ve.CanSet() {
		return 0, fmt.Errorf("can't decode to unsettable %s", ve.Type().String())
	}
	return s.decode(ve)
}

func (s *Decoder) decode(v reflect.Value) (int, error) {
	if !v.IsValid() {
		return 0, fmt.Errorf("type %s is invalid", v.Kind().String())
	}

	val, err := s.indirect(v)

	if err != nil {
		return 0, err
	}

	if ecdc, ok := val.Interface().(EncodeDecode); ok {
		return ecdc.Decode(s)
	}

	if v, ok := customPairs[val.Type().String()]; ok {
		return v.Decode(s, val)
	}

	switch val.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32:
		i, n, err := s.DecodeInt()
		if err != nil {
			return n, err
		}
		if val.OverflowInt(int64(i)) {
			return n, fmt.Errorf("signed integer too large for %s", val.Kind().String())
		}
		val.SetInt(int64(i))
		return n, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32:
		i, n, err := s.DecodeUint()
		if err != nil {
			return n, err
		}
		if val.OverflowUint(uint64(i)) {
			return n, fmt.Errorf("unsigned integer too large for %s", val.Kind().String())
		}
		val.SetUint(uint64(i))
		return n, nil
	case reflect.Int64:
		i, n, err := s.DecodeHyper()
		if err != nil {
			return n, err
		}
		val.SetInt(i)
		return n, nil
	case reflect.Uint64:
		i, n, err := s.DecodeUhyper()
		if err != nil {
			return n, err
		}
		val.SetUint(i)
		return n, nil
	case reflect.Bool:
		b, n, err := s.DecodeBool()
		if err != nil {
			return n, err
		}
		val.SetBool(b)
		return n, nil
	case reflect.Float32:
		f, n, err := s.DecodeFloat()
		if err != nil {
			return n, err
		}
		val.SetFloat(float64(f))
		return n, nil
	case reflect.Float64:
		f, n, err := s.DecodeDouble()
		if err != nil {
			return n, err
		}
		val.SetFloat(f)
		return n, nil
	case reflect.String:
		st, n, err := s.DecodeString()
		if err != nil {
			return n, err
		}
		val.SetString(st)
		return n, nil
	case reflect.Array:
		n, err := s.decodeFixedArray(val)
		if err != nil {
			return n, err
		}
		return n, nil
	case reflect.Slice:
		n, err := s.decodeArray(val)
		if err != nil {
			return n, err
		}
		return n, nil
	case reflect.Struct:
		n, err := s.decodeStruct(val)
		if err != nil {
			return n, err
		}
		return n, nil

	case reflect.Map:
		n, err := s.decodeMap(val)
		if err != nil {
			return n, err
		}
		return n, nil

	case reflect.Interface:
		n, err := s.decodeInterface(val)
		if err != nil {
			return n, err
		}
		return n, nil
	}
	return 0, fmt.Errorf("unsupported Go type %s", val.Kind().String())
}
