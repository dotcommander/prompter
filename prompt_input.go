package main

// promptInput owns one invocation's source, boundary envelope and lazy token
// estimate. It is consumed sequentially by routing, preflight and generation.
type promptInput struct {
	source        string
	bounded       string
	tokenEstimate int
}

func (input *promptInput) bound() string {
	if input.bounded == "" {
		input.bounded = boundPromptInput(input.source)
	}
	return input.bounded
}

func (input *promptInput) estimatedTokens() int {
	// The nonempty boundary envelope always has a positive token estimate.
	if input.tokenEstimate == 0 {
		input.tokenEstimate = estimateTokens(input.bound())
	}
	return input.tokenEstimate
}
