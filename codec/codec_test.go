package codec

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

type testStruct struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
	Data  []byte `json:"data"`
}

type nestedStruct struct {
	ID    string            `json:"id"`
	Inner testStruct        `json:"inner"`
	List  []int             `json:"list"`
	Map   map[string]string `json:"map"`
}

func TestJSONCodec_Marshal(t *testing.T) {
	codec := &JSONCodec{}

	tests := []struct {
		name    string
		version CodecVersion
		input   interface{}
		wantErr bool
	}{
		{
			name:    "marshal simple struct",
			version: CurrentVersion,
			input: testStruct{
				Name:  "test",
				Value: 42,
				Data:  []byte("hello"),
			},
			wantErr: false,
		},
		{
			name:    "marshal nested struct",
			version: CurrentVersion,
			input: nestedStruct{
				ID: "test-id",
				Inner: testStruct{
					Name:  "inner",
					Value: 100,
					Data:  []byte("world"),
				},
				List: []int{1, 2, 3},
				Map:  map[string]string{"key": "value"},
			},
			wantErr: false,
		},
		{
			name:    "marshal nil",
			version: CurrentVersion,
			input:   nil,
			wantErr: false,
		},
		{
			name:    "marshal empty struct",
			version: CurrentVersion,
			input:   testStruct{},
			wantErr: false,
		},
		{
			name:    "marshal string",
			version: CurrentVersion,
			input:   "test string",
			wantErr: false,
		},
		{
			name:    "marshal number",
			version: CurrentVersion,
			input:   123.456,
			wantErr: false,
		},
		{
			name:    "marshal bool",
			version: CurrentVersion,
			input:   true,
			wantErr: false,
		},
		{
			name:    "marshal slice",
			version: CurrentVersion,
			input:   []string{"a", "b", "c"},
			wantErr: false,
		},
		{
			name:    "marshal map",
			version: CurrentVersion,
			input:   map[string]int{"one": 1, "two": 2},
			wantErr: false,
		},
		{
			name:    "unsupported version",
			version: CodecVersion(999),
			input:   testStruct{Name: "test"},
			wantErr: true,
		},
		{
			name:    "marshal channel (should fail)",
			version: CurrentVersion,
			input:   make(chan int),
			wantErr: true,
		},
		{
			name:    "marshal function (should fail)",
			version: CurrentVersion,
			input:   func() {},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := codec.Marshal(tt.version, tt.input)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)

				// For valid marshals, verify it's valid JSON
				if tt.input != nil {
					var result interface{}
					err := json.Unmarshal(data, &result)
					require.NoError(t, err)
				}
			}
		})
	}
}

func TestJSONCodec_Unmarshal(t *testing.T) {
	codec := &JSONCodec{}

	tests := []struct {
		name     string
		input    []byte
		target   interface{}
		wantErr  bool
		validate func(t *testing.T, v interface{})
	}{
		{
			name:    "unmarshal simple struct",
			input:   []byte(`{"name":"test","value":42,"data":"aGVsbG8="}`),
			target:  &testStruct{},
			wantErr: false,
			validate: func(t *testing.T, v interface{}) {
				s := v.(*testStruct)
				require.Equal(t, "test", s.Name)
				require.Equal(t, 42, s.Value)
				require.Equal(t, []byte("hello"), s.Data)
			},
		},
		{
			name: "unmarshal nested struct",
			input: []byte(`{
				"id":"test-id",
				"inner":{"name":"inner","value":100,"data":"d29ybGQ="},
				"list":[1,2,3],
				"map":{"key":"value"}
			}`),
			target:  &nestedStruct{},
			wantErr: false,
			validate: func(t *testing.T, v interface{}) {
				s := v.(*nestedStruct)
				require.Equal(t, "test-id", s.ID)
				require.Equal(t, "inner", s.Inner.Name)
				require.Equal(t, 100, s.Inner.Value)
				require.Equal(t, []int{1, 2, 3}, s.List)
				require.Equal(t, "value", s.Map["key"])
			},
		},
		{
			name:    "unmarshal string",
			input:   []byte(`"test string"`),
			target:  new(string),
			wantErr: false,
			validate: func(t *testing.T, v interface{}) {
				s := v.(*string)
				require.Equal(t, "test string", *s)
			},
		},
		{
			name:    "unmarshal number",
			input:   []byte(`123.456`),
			target:  new(float64),
			wantErr: false,
			validate: func(t *testing.T, v interface{}) {
				n := v.(*float64)
				require.Equal(t, 123.456, *n)
			},
		},
		{
			name:    "unmarshal bool",
			input:   []byte(`true`),
			target:  new(bool),
			wantErr: false,
			validate: func(t *testing.T, v interface{}) {
				b := v.(*bool)
				require.True(t, *b)
			},
		},
		{
			name:    "unmarshal array",
			input:   []byte(`["a","b","c"]`),
			target:  &[]string{},
			wantErr: false,
			validate: func(t *testing.T, v interface{}) {
				s := v.(*[]string)
				require.Equal(t, []string{"a", "b", "c"}, *s)
			},
		},
		{
			name:    "unmarshal null",
			input:   []byte(`null`),
			target:  new(interface{}),
			wantErr: false,
			validate: func(t *testing.T, v interface{}) {
				p := v.(*interface{})
				require.Nil(t, *p)
			},
		},
		{
			name:    "unmarshal empty object",
			input:   []byte(`{}`),
			target:  &testStruct{},
			wantErr: false,
			validate: func(t *testing.T, v interface{}) {
				s := v.(*testStruct)
				require.Empty(t, s.Name)
				require.Zero(t, s.Value)
				require.Nil(t, s.Data)
			},
		},
		{
			name:    "unmarshal invalid JSON",
			input:   []byte(`{invalid json`),
			target:  &testStruct{},
			wantErr: true,
		},
		{
			name:    "unmarshal type mismatch",
			input:   []byte(`"string value"`),
			target:  new(int),
			wantErr: true,
		},
		{
			name:    "unmarshal empty input",
			input:   []byte(``),
			target:  &testStruct{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			version, err := codec.Unmarshal(tt.input, tt.target)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, CurrentVersion, version)

				if tt.validate != nil {
					tt.validate(t, tt.target)
				}
			}
		})
	}
}

func TestJSONCodec_RoundTrip(t *testing.T) {
	codec := &JSONCodec{}

	tests := []struct {
		name  string
		input interface{}
	}{
		{
			name: "simple struct",
			input: testStruct{
				Name:  "roundtrip",
				Value: 999,
				Data:  []byte("test data"),
			},
		},
		{
			name: "nested struct",
			input: nestedStruct{
				ID: "nested-id",
				Inner: testStruct{
					Name:  "inner-test",
					Value: 777,
					Data:  []byte("inner data"),
				},
				List: []int{10, 20, 30},
				Map:  map[string]string{"foo": "bar", "baz": "qux"},
			},
		},
		{
			name: "slice of structs",
			input: []testStruct{
				{Name: "first", Value: 1},
				{Name: "second", Value: 2},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal
			data, err := codec.Marshal(CurrentVersion, tt.input)
			require.NoError(t, err)

			// Create new instance of same type for unmarshaling
			targetType := reflect.TypeOf(tt.input)
			target := reflect.New(targetType).Interface()

			// Unmarshal
			version, err := codec.Unmarshal(data, target)
			require.NoError(t, err)
			require.Equal(t, CurrentVersion, version)

			// Compare
			require.Equal(t, tt.input, reflect.ValueOf(target).Elem().Interface())
		})
	}
}

func TestCodecVersion(t *testing.T) {
	// Test that CurrentVersion is 0
	require.Equal(t, CodecVersion(0), CurrentVersion)

	// Test version comparison
	require.True(t, CurrentVersion == 0)
	require.False(t, CurrentVersion != 0)
}

func TestCodec(t *testing.T) {
	// Test that global Codec variable is initialized
	require.NotNil(t, Codec)
	require.IsType(t, &JSONCodec{}, Codec)

	// Test using global codec
	input := testStruct{Name: "global", Value: 100}
	data, err := Codec.Marshal(CurrentVersion, input)
	require.NoError(t, err)

	var result testStruct
	version, err := Codec.Unmarshal(data, &result)
	require.NoError(t, err)
	require.Equal(t, CurrentVersion, version)
	require.Equal(t, input, result)
}

func BenchmarkJSONCodec_Marshal(b *testing.B) {
	codec := &JSONCodec{}
	input := nestedStruct{
		ID: "bench-id",
		Inner: testStruct{
			Name:  "benchmark",
			Value: 42,
			Data:  []byte("benchmark data"),
		},
		List: []int{1, 2, 3, 4, 5},
		Map:  map[string]string{"key1": "value1", "key2": "value2"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = codec.Marshal(CurrentVersion, input)
	}
}

func BenchmarkJSONCodec_Unmarshal(b *testing.B) {
	codec := &JSONCodec{}
	data := []byte(`{
		"id":"bench-id",
		"inner":{"name":"benchmark","value":42,"data":"YmVuY2htYXJrIGRhdGE="},
		"list":[1,2,3,4,5],
		"map":{"key1":"value1","key2":"value2"}
	}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var result nestedStruct
		_, _ = codec.Unmarshal(data, &result)
	}
}

func BenchmarkJSONCodec_RoundTrip(b *testing.B) {
	codec := &JSONCodec{}
	input := testStruct{
		Name:  "benchmark",
		Value: 42,
		Data:  []byte("benchmark data"),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		data, _ := codec.Marshal(CurrentVersion, input)
		var result testStruct
		_, _ = codec.Unmarshal(data, &result)
	}
}
