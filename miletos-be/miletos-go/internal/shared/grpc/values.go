package grpcserver

import (
	"time"

	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func Struct(value map[string]any) *structpb.Struct {
	if value == nil {
		return nil
	}
	converted, err := structpb.NewStruct(value)
	if err != nil {
		return &structpb.Struct{}
	}
	return converted
}

func Time(value time.Time) *timestamppb.Timestamp {
	return timestamppb.New(value)
}

func OptionalTime(value *time.Time) *timestamppb.Timestamp {
	if value == nil {
		return nil
	}
	return timestamppb.New(*value)
}
