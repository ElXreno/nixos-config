package ui

import "context"

type graphNode struct {
	Node
	X *float32 `json:"px,omitempty"`
	Y *float32 `json:"py,omitempty"`
}

type graphTopic struct {
	TopicOut
	X    *float32  `json:"x,omitempty"`
	Y    *float32  `json:"y,omitempty"`
	R    float32   `json:"R,omitempty"`
	Cap  int       `json:"cap,omitempty"`
	Hull []float32 `json:"hull,omitempty"`
	LX   *float32  `json:"lx,omitempty"`
	LY   *float32  `json:"ly,omitempty"`
}

type graphOut struct {
	*Graph
	Topics  []graphTopic `json:"topics"`
	Records []graphNode  `json:"records"`
	Layout  layoutOut    `json:"layout"`
	Rev     string       `json:"rev,omitempty"`
}

func (s *Server) graphOut(ctx context.Context, g *Graph) graphOut {
	view, state := s.layoutFor(ctx, g)
	out := graphOut{Graph: g, Topics: make([]graphTopic, len(g.Topics)), Records: make([]graphNode, len(g.Records))}
	for i, t := range g.Topics {
		out.Topics[i] = graphTopic{TopicOut: t}
	}
	for i, n := range g.Records {
		out.Records[i] = graphNode{Node: n}
	}
	if view == nil {
		out.Layout = *state
		return out
	}
	out.Layout = view.out
	for i, t := range g.Topics {
		geo, ok := view.geom[t.ID]
		if !ok {
			continue
		}
		x, y, lx, ly := float32(geo.X), float32(geo.Y), float32(geo.LX), float32(geo.LY)
		hull := make([]float32, len(geo.Hull))
		for k, h := range geo.Hull {
			hull[k] = float32(h)
		}
		out.Topics[i] = graphTopic{TopicOut: t, X: &x, Y: &y, R: float32(geo.R), Cap: geo.Cap, Hull: hull, LX: &lx, LY: &ly}
	}
	for i := range g.Records {
		if !view.has[i] {
			continue
		}
		x, y := float32(view.pos[i].X), float32(view.pos[i].Y)
		out.Records[i].X, out.Records[i].Y = &x, &y
	}
	return out
}
