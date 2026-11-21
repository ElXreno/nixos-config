package vec

import (
	"fmt"
	"math"
	"slices"
)

type DiffResult struct {
	Part  string  `json:"part"`
	Count int     `json:"count"`
	Mean  float64 `json:"mean"`
	P1    float64 `json:"p1"`
	Min   float64 `json:"min"`
}

func Diff(dirA, dirB string, parts []Part) ([]DiffResult, error) {
	var out []DiffResult
	for _, p := range parts {
		a, err := Load(dirA, p)
		if err != nil {
			return nil, err
		}
		b, err := Load(dirB, p)
		if err != nil {
			return nil, err
		}
		if a.Dim != b.Dim {
			return nil, fmt.Errorf("%s: dimension %d against %d", p.Name, a.Dim, b.Dim)
		}
		if len(a.Names) != len(b.Names) {
			return nil, fmt.Errorf("%s: %d keys against %d", p.Name, len(a.Names), len(b.Names))
		}
		at := make(map[string]int, len(b.Names))
		for i, n := range b.Names {
			at[n] = i
		}
		cos := make([]float64, 0, len(a.Names))
		for i, n := range a.Names {
			j, ok := at[n]
			if !ok {
				return nil, fmt.Errorf("%s: key %s is missing from the second run", p.Name, n)
			}
			cos = append(cos, Dot(a.Row(i), b.Row(j)))
		}
		slices.Sort(cos)
		var sum float64
		for _, c := range cos {
			sum += c
		}
		r := DiffResult{Part: p.Name, Count: len(cos), Mean: sum / float64(len(cos)), Min: cos[0]}
		r.P1 = cos[int(math.Floor(0.01*float64(len(cos)-1)))]
		out = append(out, r)
	}
	return out, nil
}
