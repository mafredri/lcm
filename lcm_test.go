package lcm

import "testing"

func testSetDisplay(t *testing.T, line DisplayLine, indent int, text string) []byte {
	b, _ := SetDisplay(line, indent, text)
	return b
}

func Test_sum(t *testing.T) {
	type args struct {
		b []byte
	}
	tests := []struct {
		name  string
		args  args
		wantS byte
	}{
		{name: "Test display status", args: args{b: []byte{0xf0, 0x01, 0x11, 0x01}}, wantS: 0x03},
		{name: "Test write spaces", args: args{b: []byte{0xf0, 0x12, 0x27, 0x00, 0x00, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20}}, wantS: 0x29},
		{name: "Test write spaces2", args: args{b: testSetDisplay(t, DisplayTop, 0, "")}, wantS: 0x29},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if gotS := checksum(tt.args.b); gotS != tt.wantS {
				t.Errorf("sum() = %#x, want %#x", gotS, tt.wantS)
			}
		})
	}
}
