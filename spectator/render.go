package spectator

import (
	"context"
	"errors"
	"math"
)

type Vector struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}
type Render struct {
	CameraMode     string `json:"cameraMode"`
	SelectionName  string `json:"selectionName"`
	CameraAttached bool   `json:"cameraAttached"`
	CameraPosition Vector `json:"cameraPosition"`
	CameraRotation Vector `json:"cameraRotation"`
	CameraOffset   Vector `json:"cameraOffset"`
}
type RenderUpdate struct {
	CameraMode     *string `json:"cameraMode,omitempty"`
	SelectionName  *string `json:"selectionName,omitempty"`
	CameraAttached *bool   `json:"cameraAttached,omitempty"`
	CameraPosition *Vector `json:"cameraPosition,omitempty"`
	CameraRotation *Vector `json:"cameraRotation,omitempty"`
	CameraOffset   *Vector `json:"cameraOffset,omitempty"`
}

func (c *Client) Render(ctx context.Context) (Render, error) {
	var render Render
	err := c.http.Request(ctx, "GET", "/replay/render", nil, &render)
	return render, err
}
func (c *Client) UpdateRender(ctx context.Context, update RenderUpdate) error {
	if update.CameraMode == nil && update.SelectionName == nil && update.CameraAttached == nil && update.CameraPosition == nil && update.CameraRotation == nil && update.CameraOffset == nil {
		return errors.New("render update is empty")
	}
	if update.CameraMode != nil && *update.CameraMode != "fps" && *update.CameraMode != "tps" {
		return errors.New("camera mode must be fps or tps")
	}
	for _, v := range []*Vector{update.CameraPosition, update.CameraRotation, update.CameraOffset} {
		if v != nil {
			for _, n := range []float64{v.X, v.Y, v.Z} {
				if math.IsNaN(n) || math.IsInf(n, 0) {
					return errors.New("camera vectors must be finite")
				}
			}
		}
	}
	return c.http.Request(ctx, "POST", "/replay/render", update, nil)
}
