package main

import (
	"context"

	"github.com/gotd/td/bin"
)

// recordingInvoker records every query it receives and answers with errs in
// order, then with nil once they run out.
type recordingInvoker struct {
	inputs []bin.Encoder
	errs   []error
}

func (r *recordingInvoker) Invoke(_ context.Context, input bin.Encoder, _ bin.Decoder) error {
	r.inputs = append(r.inputs, input)
	if len(r.errs) == 0 {
		return nil
	}

	err := r.errs[0]
	r.errs = r.errs[1:]

	return err
}
