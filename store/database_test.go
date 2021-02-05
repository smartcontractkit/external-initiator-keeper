package store

import (
	"database/sql/driver"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSQLStringArray_Scan(t *testing.T) {
	type args struct {
		src interface{}
	}
	tests := []struct {
		name    string
		arr     SQLStringArray
		args    args
		wantErr bool
		result  []string
	}{
		{
			"splits comma delimited string",
			SQLStringArray{},
			args{"abc,123"},
			false,
			[]string{"abc", "123"},
		},
		{
			"fails on invalid list",
			SQLStringArray{},
			args{`a""b,c`},
			true,
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.arr.Scan(tt.args.src); (err != nil) != tt.wantErr {
				t.Errorf("Scan() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				for i := range tt.result {
					assert.Equal(t, tt.result[i], tt.arr[i])
				}
			}
		})
	}
}

func TestSQLStringArray_Value(t *testing.T) {
	tests := []struct {
		name    string
		arr     SQLStringArray
		want    driver.Value
		wantErr bool
	}{
		{
			"turns string slice into csv list",
			SQLStringArray{"abc", "123"},
			"abc,123\n",
			false,
		},
		{
			"empty input gives empty output",
			SQLStringArray{},
			"\n",
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.arr.Value()
			if (err != nil) != tt.wantErr {
				t.Errorf("Value() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Value() got = %v, want %v", got, tt.want)
			}
		})
	}
}
