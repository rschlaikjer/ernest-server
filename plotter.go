package main

import (
	"code.google.com/p/plotinum/plot"
	"code.google.com/p/plotinum/plotter"
	"code.google.com/p/plotinum/vg"
	"fmt"
	"image/color"
	"time"
)

func generateTempPlot(d *Decider, farenheit bool, outfile string) error {
	p, err := plot.New()
	if err != nil {
		return err
	}

	p.Title.Text = "Ernest Node Temps"
	p.X.Label.Text = "Date"
	p.Y.Label.Text = "Temperature"
	p.Add(plotter.NewGrid())
	p.X.Tick.Marker = dateTicks
	p.Y.Tick.Marker = tempTicks

	for node_id, node_data := range d.getReadingHistory() {
		node_plot_options := d.getNodePlotOpts(node_id)
		l, err := plotter.NewLine(tempDataSeries(node_data, farenheit))
		if err != nil {
			return err
		}
		l.LineStyle.Color = color.RGBA{
			R: node_plot_options.Graph_r,
			G: node_plot_options.Graph_g,
			B: node_plot_options.Graph_b,
			A: 255,
		}
		l.LineStyle.Width = vg.Points(1)
		p.Add(l)
		p.Legend.Add(node_plot_options.Name, l)
	}

	if err := p.Save(15, 10, outfile); err != nil {
		return err
	}

	return nil
}

func tempTicks(min, max float64) []plot.Tick {
	tks := plot.DefaultTicks(min, max)
	for i, t := range tks {
		//if t.Label == "" {
		//	continue
		//}
		tks[i].Label = fmt.Sprintf("%0.2f", t.Value)
	}
	return tks
}

func dateTicks(min, max float64) []plot.Tick {
	tks := plot.DefaultTicks(min, max)
	for i, t := range tks {
		timestamp := time.Unix(int64(t.Value), 0)
		tks[i].Label = fmt.Sprintf("%s", timestamp)
	}
	return tks
}

func tempDataSeries(node_data []*ReadingData, f bool) plotter.XYs {
	pts := make(plotter.XYs, len(node_data))
	for i, reading := range node_data {
		pts[i].X = float64(reading.Time)
		pts[i].Y = reading.Temp.Float64
		if f {
			pts[i].Y = pts[i].Y*(9.0/5.0) + 32
		}
	}
	return pts
}
