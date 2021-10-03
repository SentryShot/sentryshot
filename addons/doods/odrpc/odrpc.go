// DOODS v0.2.1 vendored. https://github.com/snowzach/doods

// The MIT License (MIT)
//
// Copyright (c) 2018 Zach Brown <zach@prozach.org>
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package odrpc

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/sortkeys"
	"google.golang.org/grpc"
)

var (
	ErrInvalidLengthRpc = fmt.Errorf("proto: negative length found during unmarshaling") //nolint:revive,stylecheck
	ErrIntOverflowRpc   = fmt.Errorf("proto: integer overflow")                          //nolint:revive,stylecheck
)

// Reference imports to suppress errors if they are not otherwise used.
var (
	_ = proto.Marshal
	_ = fmt.Errorf
	_ = math.Inf
)

// Reference imports to suppress errors if they are not otherwise used.
var (
	_ context.Context
	_ grpc.ClientConn
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
const _ = grpc.SupportPackageIsVersion4

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.GoGoProtoPackageIsVersion2 // please upgrade the proto package

type GetDetectorsResponse struct {
	Detectors []*Detector `protobuf:"bytes,1,rep,name=detectors,proto3" json:"detectors,omitempty"`
}

func (m *GetDetectorsResponse) Reset()      { *m = GetDetectorsResponse{} }
func (*GetDetectorsResponse) ProtoMessage() {}

type Detector struct {
	// The name for this config
	Name string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	// The name for this config
	Type string `protobuf:"bytes,2,opt,name=type,proto3" json:"type,omitempty"`
	// Model Name
	Model string `protobuf:"bytes,3,opt,name=model,proto3" json:"model,omitempty"`
	// Labels
	Labels []string `protobuf:"bytes,4,rep,name=labels,proto3" json:"labels,omitempty"`
	// The detection width
	Width int32 `protobuf:"varint,5,opt,name=width,proto3" json:"width,omitempty"`
	// The detection height
	Height int32 `protobuf:"varint,6,opt,name=height,proto3" json:"height,omitempty"`
	// The detection channels
	Channels int32 `protobuf:"varint,7,opt,name=channels,proto3" json:"channels,omitempty"`
}

func (m *Detector) Reset()      { *m = Detector{} }
func (*Detector) ProtoMessage() {}

func (m *Detector) GetName() string {
	if m != nil {
		return m.Name
	}
	return ""
}

func (m *Detector) GetType() string {
	if m != nil {
		return m.Type
	}
	return ""
}

func (m *Detector) GetModel() string {
	if m != nil {
		return m.Model
	}
	return ""
}

func (m *Detector) GetLabels() []string {
	if m != nil {
		return m.Labels
	}
	return nil
}

func (m *Detector) GetWidth() int32 {
	if m != nil {
		return m.Width
	}
	return 0
}

func (m *Detector) GetHeight() int32 {
	if m != nil {
		return m.Height
	}
	return 0
}

func (m *Detector) GetChannels() int32 {
	if m != nil {
		return m.Channels
	}
	return 0
}

// The Process Request.
type DetectRequest struct {
	// The ID for the request.
	Id string `protobuf:"bytes,1,opt,name=id,proto3" json:"id,omitempty"` //nolint:revive,stylecheck
	// The ID for the request.
	DetectorName string `protobuf:"bytes,2,opt,name=detector_name,json=detectorName,proto3" json:"detector_name,omitempty"`
	// The image data
	Data Raw `protobuf:"bytes,3,opt,name=data,proto3,casttype=Raw" json:"data"`
	// A filename
	File string `protobuf:"bytes,4,opt,name=file,proto3" json:"file,omitempty"`
	// What to detect
	Detect map[string]float32 `protobuf:"bytes,5,rep,name=detect,proto3" json:"detect,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"fixed32,2,opt,name=value,proto3"` //nolint:lll
	// Sub regions for detection
	Regions []*DetectRegion `protobuf:"bytes,6,rep,name=regions,proto3" json:"regions,omitempty"`
}

func (r *DetectRequest) Reset()      { *r = DetectRequest{} }
func (*DetectRequest) ProtoMessage() {}

func (r *DetectRequest) String() string {
	if r == nil {
		return "nil"
	}
	keysForDetect := make([]string, 0, len(r.Detect))
	for k := range r.Detect {
		keysForDetect = append(keysForDetect, k)
	}
	sortkeys.Strings(keysForDetect)
	mapStringForDetect := "map[string]float32{"
	for _, k := range keysForDetect {
		mapStringForDetect += fmt.Sprintf("%v: %v,", k, r.Detect[k])
	}
	mapStringForDetect += "}"

	s := strings.Join([]string{
		`&DetectRequest{`,
		`Id:` + fmt.Sprintf("%v", r.Id) + `,`,
		`DetectorName:` + fmt.Sprintf("%v", r.DetectorName) + `,`,
		`Data:` + fmt.Sprintf("%v", r.Data) + `,`,
		`File:` + fmt.Sprintf("%v", r.File) + `,`,
		`Detect:` + mapStringForDetect + `,`,
		`Regions:` + strings.Replace(fmt.Sprintf("%v", r.Regions), "DetectRegion", "DetectRegion", 1) + `,`, //nolint:gocritic
		`}`,
	}, "")
	return s
}

type DetectRegion struct {
	// Coordinates
	Top    float32 `protobuf:"fixed32,1,opt,name=top,proto3" json:"top"`
	Left   float32 `protobuf:"fixed32,2,opt,name=left,proto3" json:"left"`
	Bottom float32 `protobuf:"fixed32,3,opt,name=bottom,proto3" json:"bottom"`
	Right  float32 `protobuf:"fixed32,4,opt,name=right,proto3" json:"right"`
	// What to detect
	Detect map[string]float32 `protobuf:"bytes,5,rep,name=detect,proto3" json:"detect,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"fixed32,2,opt,name=value,proto3"` //nolint:lll
	Covers bool               `protobuf:"varint,6,opt,name=covers,proto3" json:"covers,omitempty"`
}

func (m *DetectRegion) Reset()      { *m = DetectRegion{} }
func (*DetectRegion) ProtoMessage() {}

// Area for detection.
type Detection struct {
	// Coordinates
	Top        float32 `protobuf:"fixed32,1,opt,name=top,proto3" json:"top"`
	Left       float32 `protobuf:"fixed32,2,opt,name=left,proto3" json:"left"`
	Bottom     float32 `protobuf:"fixed32,3,opt,name=bottom,proto3" json:"bottom"`
	Right      float32 `protobuf:"fixed32,4,opt,name=right,proto3" json:"right"`
	Label      string  `protobuf:"bytes,5,opt,name=label,proto3" json:"label"`
	Confidence float32 `protobuf:"fixed32,6,opt,name=confidence,proto3" json:"confidence"`
}

func (m *Detection) Reset()      { *m = Detection{} }
func (*Detection) ProtoMessage() {}

func (m *Detection) GetTop() float32 {
	if m != nil {
		return m.Top
	}
	return 0
}

func (m *Detection) GetLeft() float32 {
	if m != nil {
		return m.Left
	}
	return 0
}

func (m *Detection) GetBottom() float32 {
	if m != nil {
		return m.Bottom
	}
	return 0
}

func (m *Detection) GetRight() float32 {
	if m != nil {
		return m.Right
	}
	return 0
}

func (m *Detection) GetLabel() string {
	if m != nil {
		return m.Label
	}
	return ""
}

func (m *Detection) GetConfidence() float32 {
	if m != nil {
		return m.Confidence
	}
	return 0
}

type DetectResponse struct {
	// The id for the response
	Id string `protobuf:"bytes,1,opt,name=id,proto3" json:"id,omitempty"` //nolint:revive,stylecheck
	// The detected areas
	Detections []*Detection `protobuf:"bytes,2,rep,name=detections,proto3" json:"detections,omitempty"`
	// If there was an error (streaming endpoint only)
	Error string `protobuf:"bytes,3,opt,name=error,proto3" json:"error,omitempty"`
}

func (r *DetectResponse) Reset()      { *r = DetectResponse{} }
func (*DetectResponse) ProtoMessage() {}

func (r *DetectResponse) GetId() string { //nolint:revive,stylecheck
	if r != nil {
		return r.Id
	}
	return ""
}

func (r *DetectResponse) GetDetections() []*Detection {
	if r != nil {
		return r.Detections
	}
	return nil
}

func (r *DetectResponse) String() string {
	if r == nil {
		return "nil"
	}
	s := strings.Join([]string{
		`&DetectResponse{`,
		`Id:` + fmt.Sprintf("%v", r.Id) + `,`,
		`Detections:` + strings.Replace(fmt.Sprintf("%v", r.Detections), "Detection", "Detection", 1) + `,`, //nolint:gocritic
		`Error:` + fmt.Sprintf("%v", r.Error) + `,`,
		`}`,
	}, "")
	return s
}

type odrpcClient struct {
	cc *grpc.ClientConn
}

func NewOdrpcClient(cc *grpc.ClientConn) *odrpcClient { //nolint:revive
	return &odrpcClient{cc}
}

func (c *odrpcClient) DetectStream(ctx context.Context, opts ...grpc.CallOption) (*OdrpcDetectStreamClient, error) {
	desc := &grpc.StreamDesc{
		StreamName:    "DetectStream",
		ServerStreams: true,
		ClientStreams: true,
	}

	stream, err := c.cc.NewStream(ctx, desc, "/odrpc.odrpc/DetectStream", opts...)
	if err != nil {
		return nil, err
	}
	x := &OdrpcDetectStreamClient{stream}
	return x, nil
}

type OdrpcDetectStreamClient struct { //nolint:revive
	grpc.ClientStream
}

func (x *OdrpcDetectStreamClient) Send(m *DetectRequest) error {
	return x.ClientStream.SendMsg(m)
}

func (x *OdrpcDetectStreamClient) Recv() (*DetectResponse, error) {
	m := new(DetectResponse)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// Images are byte arrays.
type Raw []byte
