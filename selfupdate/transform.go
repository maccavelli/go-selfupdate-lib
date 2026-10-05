package selfupdate

import (
	"context"
	"errors"
	"fmt"
)

type noopTransformer struct{}

func (noopTransformer) Transform(context.Context, TransformRequest) error {
	return nil
}

type transformerChain []Transformer

// ChainTransformers returns a Transformer that runs ts in order on the same
// request, and stops at the first error. The coordinator checks and
// rehashes the staging file once, after the whole chain. It refuses an
// empty list, and a nil or typed-nil element (0012-MADR §7).
func ChainTransformers(ts ...Transformer) (Transformer, error) {
	if len(ts) == 0 {
		return nil, errors.New("selfupdate: a transformer chain needs at least one transformer")
	}
	for i, t := range ts {
		if isNil(t) {
			return nil, fmt.Errorf("selfupdate: transformer %d of the chain is nil", i)
		}
	}
	return append(transformerChain(nil), ts...), nil
}

// Transform implements Transformer.
func (c transformerChain) Transform(ctx context.Context, r TransformRequest) error {
	for _, t := range c {
		if err := t.Transform(ctx, r); err != nil {
			return err
		}
	}
	return nil
}
