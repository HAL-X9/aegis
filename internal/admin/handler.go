package admin

import "net/http"

type Probe interface {
	Liveness() error
}

type SystemHandler struct {
}

func NewSystemHandler(probe Probe) *SystemHandler {
	return nil
}

func (handler *SystemHandler) Handler() http.Handler {
	return nil
}
