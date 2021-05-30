package lcm

import (
	"bytes"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func Test_recvMessage_WriteByte(t *testing.T) {
	type want struct {
		sum byte
		len byte
	}
	type args struct {
		b []byte
	}
	tests := []struct {
		name    string
		args    args
		want    *want
		wantErr bool
	}{
		{
			name: "Good message",
			args: args{b: []byte{0xf1, 0x01, 0x12, 0x00, 0x04}},
			want: &want{
				sum: 0x04,
				len: 4,
			},
		},
		{
			name:    "Invalid checksum",
			args:    args{b: []byte{0xf1, 0x01, 0x12, 0x00, 0x00}},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &recvMessage{}
			if err := copyBytes(m, bytes.NewBuffer(tt.args.b)); (err != nil) != tt.wantErr {
				t.Errorf("recvMessage.WriteByte() error: wantErr %v, got %v", tt.wantErr, err)
			}
			if tt.want != nil {
				if m.len != tt.want.len {
					t.Errorf("recvMessage.WriteByte() len: want %v, got %v", tt.want.len, m.len)
				}
				if m.sum != tt.want.sum {
					t.Errorf("recvMessage.WriteByte() sum: want %v, got %v", tt.want.sum, m.sum)
				}
				if diff := cmp.Diff(tt.args.b, m.buf.Bytes()); diff != "" {
					t.Errorf("recvMessage.WriteByte() buf (-want +got)\n%s", diff)
				}
			}
		})
	}
}
