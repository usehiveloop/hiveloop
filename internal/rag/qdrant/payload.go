package qdrant

import (
	qc "github.com/qdrant/go-client/qdrant"
)

func toValueMap(m map[string]any) map[string]*qc.Value {
	if m == nil {
		return nil
	}
	out := make(map[string]*qc.Value, len(m))
	for k, v := range m {
		out[k] = toValue(v)
	}
	return out
}

func toValue(v any) *qc.Value {
	switch x := v.(type) {
	case nil:
		return &qc.Value{Kind: &qc.Value_NullValue{}}
	case string:
		return &qc.Value{Kind: &qc.Value_StringValue{StringValue: x}}
	case bool:
		return &qc.Value{Kind: &qc.Value_BoolValue{BoolValue: x}}
	case int:
		return &qc.Value{Kind: &qc.Value_IntegerValue{IntegerValue: int64(x)}}
	case int32:
		return &qc.Value{Kind: &qc.Value_IntegerValue{IntegerValue: int64(x)}}
	case int64:
		return &qc.Value{Kind: &qc.Value_IntegerValue{IntegerValue: x}}
	case uint32:
		return &qc.Value{Kind: &qc.Value_IntegerValue{IntegerValue: int64(x)}}
	case float32:
		return &qc.Value{Kind: &qc.Value_DoubleValue{DoubleValue: float64(x)}}
	case float64:
		return &qc.Value{Kind: &qc.Value_DoubleValue{DoubleValue: x}}
	case []string:
		vals := make([]*qc.Value, len(x))
		for i, s := range x {
			vals[i] = &qc.Value{Kind: &qc.Value_StringValue{StringValue: s}}
		}
		return &qc.Value{Kind: &qc.Value_ListValue{ListValue: &qc.ListValue{Values: vals}}}
	case []any:
		vals := make([]*qc.Value, len(x))
		for i, item := range x {
			vals[i] = toValue(item)
		}
		return &qc.Value{Kind: &qc.Value_ListValue{ListValue: &qc.ListValue{Values: vals}}}
	case map[string]any:
		return &qc.Value{Kind: &qc.Value_StructValue{StructValue: &qc.Struct{Fields: toValueMap(x)}}}
	case map[string]string:
		fields := make(map[string]*qc.Value, len(x))
		for k, s := range x {
			fields[k] = &qc.Value{Kind: &qc.Value_StringValue{StringValue: s}}
		}
		return &qc.Value{Kind: &qc.Value_StructValue{StructValue: &qc.Struct{Fields: fields}}}
	default:
		return &qc.Value{Kind: &qc.Value_NullValue{}}
	}
}

func fromValueMap(m map[string]*qc.Value) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = fromValue(v)
	}
	return out
}

func fromValue(v *qc.Value) any {
	if v == nil {
		return nil
	}
	switch k := v.Kind.(type) {
	case *qc.Value_StringValue:
		return k.StringValue
	case *qc.Value_BoolValue:
		return k.BoolValue
	case *qc.Value_IntegerValue:
		return k.IntegerValue
	case *qc.Value_DoubleValue:
		return k.DoubleValue
	case *qc.Value_ListValue:
		out := make([]any, len(k.ListValue.GetValues()))
		for i, item := range k.ListValue.GetValues() {
			out[i] = fromValue(item)
		}
		return out
	case *qc.Value_StructValue:
		return fromValueMap(k.StructValue.GetFields())
	}
	return nil
}
