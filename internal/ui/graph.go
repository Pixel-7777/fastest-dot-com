package ui

import (
	"github.com/guptarohit/asciigraph"
)

type Graph struct {
	Data   []float64
	Width  int
	Height int
	Title  string
}

// initializes a new graph filled with zeros
func NewGraph(title string, width, height int) *Graph {
	return &Graph{
		Data:   make([]float64, width), // Pad with zeros to start flat
		Width:  width,
		Height: height,
		Title:  title,
	}
}

// shifts old data out and adds new reading
func (g *Graph) AddPoint(val float64) {
	g.Data = append(g.Data[1:], val)
}

func (g *Graph) View() string {
	return asciigraph.Plot(g.Data,
		asciigraph.Height(g.Height),
		asciigraph.Width(g.Width),
		asciigraph.Caption(g.Title),
	)
}
