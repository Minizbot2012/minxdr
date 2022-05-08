package minxdr

import "reflect"

type EncDecPair interface {
	Encode(*Encoder, reflect.Value) (int, error)
	Decode(*Decoder, reflect.Value) (int, error)
}

type EncodeDecode interface {
	Encode(*Encoder) (int, error)
	Decode(*Decoder) (int, error)
}
