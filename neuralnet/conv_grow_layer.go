package neuralnet

import (
	"bytes"
	"encoding/binary"
)

var convGrowByteOrder = binary.LittleEndian

// ConvGrowParams stores configuration
// parameters for a ConvGrowLayer.
//
// The ConvParams parameters specify how
// the convolutional part of the layer
// should be structured.
//
// The InverseStride parameter specifies
// how many times the convolutional layer
// should be copied for adjacent pixels.
// For instance, if InverseStride is 2,
// then 2*2*FilterCount filters will be
// created.
//
// If the regular convolutional layer
// created with ConvParams would have
// output size NxM, then the corresponding
// ConvGrowLayer would have size
// (N*InverseStride)x(M*InverseStride).
type ConvGrowParams struct {
	ConvParams
	InverseStride int
}

// Make creates a new ConvGrowLayer
// using the configuration in p.
// This is equivalent to NewConvGrowParams(p).
func (p *ConvGrowParams) Make() Layer {
	return NewConvGrowLayer(p)
}

// A ConvGrowLayer is a convolutional layer
// that has a sort of "fractional" stride.
// In other words, it is a convolutional layer
// for which one input region is mapped to
// multiple adjacent output regions.
//
// This is useful for producing an output that
// is larger than the input, hence the name
// "grow" layer.
type ConvGrowLayer struct {
	output     *Tensor3
	downstream *Tensor3

	convLayer      *ConvLayer
	convDownstream *Tensor3

	inverseStride int
}

func NewConvGrowLayer(p *ConvGrowParams) *ConvGrowLayer {
	convParams := p.ConvParams
	convParams.FilterCount *= p.InverseStride * p.InverseStride
	convLayer := NewConvLayer(&convParams)

	res := &ConvGrowLayer{
		output: NewTensor3(convLayer.output.Width*p.InverseStride,
			convLayer.output.Height*p.InverseStride, p.FilterCount),

		convLayer: convLayer,
		convDownstream: NewTensor3(convLayer.output.Width, convLayer.output.Height,
			convLayer.output.Depth),

		inverseStride: p.InverseStride,
	}

	res.convLayer.SetDownstreamGradient(res.convDownstream.Data)

	return res
}

func DeserializeConvGrowLayer(data []byte) (*ConvGrowLayer, error) {
	buf := bytes.NewBuffer(data)

	var inverseStride uint32
	if err := binary.Read(buf, convGrowByteOrder, &inverseStride); err != nil {
		return nil, err
	}

	convLayer, err := DeserializeConvLayer(buf.Bytes())
	if err != nil {
		return nil, err
	}

	filterCount := len(convLayer.filters) / int(inverseStride*inverseStride)

	res := &ConvGrowLayer{
		output: NewTensor3(convLayer.output.Width*int(inverseStride),
			convLayer.output.Height*int(inverseStride), filterCount),

		convLayer: convLayer,
		convDownstream: NewTensor3(convLayer.output.Width, convLayer.output.Height,
			convLayer.output.Depth),

		inverseStride: int(inverseStride),
	}

	res.convLayer.SetDownstreamGradient(res.convDownstream.Data)

	return res, nil
}

// ConvLayer returns the underlying convolutional
// layer that drives this ConvGrowLayer.
// The layer will have InverseStride^2 * FilterCount
// filters, since it requires a set of filters for
// each of the outputs generated by a given input
// region.
func (c *ConvGrowLayer) ConvLayer() *ConvLayer {
	return c.convLayer
}

func (c *ConvGrowLayer) Randomize() {
	c.convLayer.Randomize()
}

func (c *ConvGrowLayer) PropagateForward() {
	c.convLayer.PropagateForward()

	for y := 0; y < c.output.Height; y++ {
		internalY := y / c.inverseStride
		for x := 0; x < c.output.Width; x++ {
			internalX := x / c.inverseStride
			internalOffset := ((x % c.inverseStride) + c.inverseStride*(y%c.inverseStride)) *
				c.output.Depth
			for z := 0; z < c.output.Depth; z++ {
				val := c.convLayer.output.Get(internalX, internalY, internalOffset+z)
				c.output.Set(x, y, z, val)
			}
		}
	}
}

func (c *ConvGrowLayer) PropagateBackward(upstream bool) {
	for y := 0; y < c.output.Height; y++ {
		internalY := y / c.inverseStride
		for x := 0; x < c.output.Width; x++ {
			internalX := x / c.inverseStride
			internalOffset := ((x % c.inverseStride) + c.inverseStride*(y%c.inverseStride)) *
				c.output.Depth
			for z := 0; z < c.output.Depth; z++ {
				val := c.downstream.Get(x, y, z)
				c.convDownstream.Set(internalX, internalY, internalOffset+z, val)
			}
		}
	}
	c.convLayer.PropagateBackward(upstream)
}

func (c *ConvGrowLayer) Output() []float64 {
	return c.output.Data
}

func (c *ConvGrowLayer) UpstreamGradient() []float64 {
	return c.convLayer.UpstreamGradient()
}

func (c *ConvGrowLayer) Input() []float64 {
	return c.convLayer.Input()
}

func (c *ConvGrowLayer) SetInput(v []float64) bool {
	return c.convLayer.SetInput(v)
}

func (c *ConvGrowLayer) DownstreamGradient() []float64 {
	return c.downstream.Data
}

func (c *ConvGrowLayer) SetDownstreamGradient(v []float64) bool {
	if len(v) != len(c.output.Data) {
		return false
	}
	c.downstream = &Tensor3{
		Width:  c.output.Width,
		Height: c.output.Height,
		Depth:  c.output.Depth,
		Data:   v,
	}
	return true
}

func (c *ConvGrowLayer) GradientMagSquared() float64 {
	return c.convLayer.GradientMagSquared()
}

func (c *ConvGrowLayer) StepGradient(f float64) {
	c.convLayer.StepGradient(f)
}

func (c *ConvGrowLayer) Alias() Layer {
	res := &ConvGrowLayer{
		output: NewTensor3(c.output.Width, c.output.Height, c.output.Depth),

		convLayer: c.convLayer.Alias().(*ConvLayer),
		convDownstream: NewTensor3(c.convDownstream.Width, c.convDownstream.Height,
			c.convDownstream.Depth),

		inverseStride: c.inverseStride,
	}
	res.convLayer.SetDownstreamGradient(res.convDownstream.Data)
	return res
}

func (c *ConvGrowLayer) Serialize() ([]byte, error) {
	var buf bytes.Buffer

	binary.Write(&buf, convGrowByteOrder, uint32(c.inverseStride))
	serialized, err := c.convLayer.Serialize()
	if err != nil {
		return nil, err
	}
	buf.Write(serialized)

	return buf.Bytes(), nil
}

func (c *ConvGrowLayer) SerializerType() string {
	return serializerTypeConvGrowLayer
}
