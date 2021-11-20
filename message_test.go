package lcm

import (
	"fmt"
	"testing"
)

func TestSetDisplay(t *testing.T) {
	type args struct {
		line   DisplayLine
		indent int
		text   string
	}
	tests := []struct {
		name    string
		args    args
		wantRaw string
		wantErr bool
	}{
		{
			name:    "Test spaces",
			args:    args{line: DisplayTop, indent: 0, text: "                "},
			wantRaw: "0xf01227000020202020202020202020202020202020",
		},
		{
			name:    "Test spaces (padding)",
			args:    args{line: DisplayTop, indent: 0, text: ""},
			wantRaw: "0xf01227000020202020202020202020202020202020",
		},
		{
			name:    "Test text",
			args:    args{line: DisplayTop, indent: 0, text: "PRESS ANY KEY TO"},
			wantRaw: "0xf012270000505245535320414e59204b455920544f",
		},
		{
			name:    "Test text indent",
			args:    args{line: DisplayTop, indent: 2, text: "PRESS ANY KEY TO"},
			wantRaw: "0xf012270002505245535320414e59204b455920544f",
		},
		{
			name:    "Test text too long",
			args:    args{line: DisplayTop, indent: 0, text: "PRESS ANY KEY TO EXPLODE"},
			wantErr: true,
		},
		{
			name:    "Test indent out of bounds",
			args:    args{line: DisplayTop, indent: 0xFF, text: "PRESS ANY KEY TO "},
			wantErr: true,
		},
		{
			name:    "Test display bottom",
			args:    args{line: DisplayBottom, indent: 0, text: ">"},
			wantRaw: "0xf0122701003e202020202020202020202020202020",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRaw, err := SetDisplay(tt.args.line, tt.args.indent, tt.args.text)
			if (err != nil) != tt.wantErr {
				t.Errorf("SetDisplay() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil && fmt.Sprintf("%#x", gotRaw) != tt.wantRaw {
				t.Errorf("SetDisplay() = %#x, want %s", gotRaw, tt.wantRaw)
			}
		})
	}
}
