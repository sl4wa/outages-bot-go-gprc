package subscriptions

import (
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func invalidField(field, description string) error {
	s, _ := status.New(codes.InvalidArgument, "invalid "+field).WithDetails(&errdetails.BadRequest{
		FieldViolations: []*errdetails.BadRequest_FieldViolation{{Field: field, Description: description}},
	})
	return s.Err()
}
