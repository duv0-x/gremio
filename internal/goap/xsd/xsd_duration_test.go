package xsd

import (
	"bytes"
	"testing"
	"time"
)

const (
	P2Y6M5DT12H35M30S = 2*Yearish + 6*Monthish + 5*Day + 12*time.Hour + 35*time.Minute + 30*time.Second
	P1DT2H            = Day + 2*time.Hour
	P20M              = 20 * Monthish
	PT20M             = 20 * time.Minute
	P0Y               = time.Duration(0)
	NegP60D           = -1 * (60 * Day)
	PT1M30_5S         = time.Minute + time.Duration(30.5*float64(time.Second))
)

func TestMarshal(t *testing.T) {
	type args struct {
		d time.Duration
	}
	tests := []struct {
		name    string
		args    args
		want    []byte
		wantErr bool
	}{
		{
			name:    "2 years, 6 months, 5 days, 12 hours, 35 minutes, 30 seconds",
			args:    args{P2Y6M5DT12H35M30S},
			wantErr: false,
			want:    []byte("P2Y6M5DT12H35M30S"),
		},
		{
			name:    "1 day, 2 hours",
			args:    args{P1DT2H},
			wantErr: false,
			want:    []byte("P1DT2H"),
		},
		{
			name:    "20 months (the number of months can be more than 12)",
			args:    args{P20M},
			want:    []byte("P1Y8M4D"),
			wantErr: false,
		},
		{
			name:    "20 minutes",
			args:    args{PT20M},
			want:    []byte("PT20M"),
			wantErr: false,
		},
		{
			name:    "0 years",
			args:    args{P0Y},
			want:    []byte("PT0S"),
			wantErr: false,
		},
		{
			name:    "minus 60 days",
			args:    args{NegP60D},
			want:    []byte("-P2M"),
			wantErr: false,
		},
		{
			name:    "1 minute, 30.5 seconds",
			args:    args{PT1M30_5S},
			want:    []byte("PT1M30.5S"),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Marshal(tt.args.d)
			if (err != nil) != tt.wantErr {
				t.Errorf("Marshal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !bytes.Equal(got, tt.want) {
				t.Errorf("Marshal() got = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestUnmarshal(t *testing.T) {
	type args struct {
		data []byte
		d    *time.Duration
	}
	tests := []struct {
		name    string
		args    args
		want    time.Duration
		wantErr bool
	}{
		{
			name:    "an empty value should not be valid",
			args:    args{[]byte{}, nil},
			wantErr: true,
		},
		{
			name:    "at least one number and designator are required",
			args:    args{[]byte("P"), nil},
			wantErr: true,
		},
		{
			name:    "missing type",
			args:    args{[]byte("PT"), nil},
			wantErr: true,
		},
		{
			name:    "minus sign must appear first",
			args:    args{[]byte("P-20M"), nil},
			wantErr: true,
		},
		{
			name:    "no time items are present, so \"T\" must not be present",
			args:    args{[]byte("P20MT"), nil},
			wantErr: true,
		},
		{
			name:    "no value is specified for months, so \"M\" must not be present",
			args:    args{[]byte("P1YM5D"), nil},
			wantErr: true,
		},
		{
			name:    "only the seconds can be expressed as a decimal",
			args:    args{[]byte("P15.5Y"), nil},
			wantErr: true,
		},
		{
			name:    "\"T\" must be present to separate days and hours",
			args:    args{[]byte("P1D2H"), nil},
			wantErr: true,
		},
		{
			name:    "\"P\" must always be present",
			args:    args{[]byte("1Y2M"), nil},
			wantErr: true,
		},
		{
			name:    "years must appear before months",
			args:    args{[]byte("P2M1Y"), nil},
			wantErr: true,
		},
		{
			name:    "at least one digit must follow the decimal point if it appears",
			args:    args{[]byte("PT15.S"), nil},
			wantErr: true,
		},
		{
			name:    "invalid data at the end",
			args:    args{[]byte("P2Y6M5DT12H35M30Stest"), nil},
			wantErr: true,
		},
		{
			name:    "invalid data at the start",
			args:    args{[]byte("testP2Y6M5DT12H35M30S"), nil},
			wantErr: true,
		},
		{
			name:    "2 years, 6 months, 5 days, 12 hours, 35 minutes, 30 seconds",
			args:    args{[]byte("P2Y6M5DT12H35M30S"), new(time.Duration)},
			want:    P2Y6M5DT12H35M30S,
			wantErr: false,
		},
		{
			name:    "1 day, 2 hours",
			args:    args{[]byte("P1DT2H"), new(time.Duration)},
			want:    P1DT2H,
			wantErr: false,
		},
		{
			name:    "20 months (the number of months can be more than 12)",
			args:    args{[]byte("P20M"), new(time.Duration)},
			want:    P20M,
			wantErr: false,
		},
		{
			name:    "20 minutes",
			args:    args{[]byte("PT20M"), new(time.Duration)},
			want:    PT20M,
			wantErr: false,
		},
		{
			name:    "20 months (0 is permitted as a number, but is not required)",
			args:    args{[]byte("P0Y20M0D"), new(time.Duration)},
			want:    P20M,
			wantErr: false,
		},
		{
			name:    "0 years",
			args:    args{[]byte("P0Y"), new(time.Duration)},
			want:    P0Y,
			wantErr: false,
		},
		{
			name:    "minus 60 days",
			args:    args{[]byte("-P60D"), new(time.Duration)},
			want:    NegP60D,
			wantErr: false,
		},
		{
			name:    "1 minute, 30.5 seconds",
			args:    args{[]byte("PT1M30.5S"), new(time.Duration)},
			want:    PT1M30_5S,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := Unmarshal(tt.args.data, tt.args.d); (err != nil) != tt.wantErr {
				t.Errorf("Unmarshal() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.want == 0 {
				return
			}
			if *tt.args.d != tt.want {
				t.Errorf("Marshal() got = %s, want %s", tt.args.d, tt.want)
			}
		})
	}
}
