package lcm

import (
	"io"
)

func copyBytes(dst io.ByteWriter, src io.ByteReader) error {
	for {
		c, err := src.ReadByte()
		if err != nil {
			return err
		}
		err = dst.WriteByte(c)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}
