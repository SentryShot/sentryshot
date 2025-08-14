# 7. Model Server Integration

This document describes how to integrate SentryShot with an external model server (like TensorFlow Serving or Triton Inference Server) using gRPC for object detection. This approach allows for offloading intensive computation to a dedicated server and using a wider variety of models.

## 1. Core Concepts

### Model Server
A model server is a specialized application designed to serve machine learning models over a network. It handles loading models, managing versions, and running inference efficiently. [TensorFlow Serving](https://www.tensorflow.org/tfx/guide/serving) and [NVIDIA Triton](https://developer.nvidia.com/nvidia-triton-inference-server) are popular examples that expose a gRPC endpoint for predictions.

### gRPC
gRPC is a high-performance, open-source universal RPC framework. Communication between the SentryShot plugin (client) and the model server is handled via gRPC calls.

### Protocol Buffers (Protobuf)
Protobuf is the Interface Definition Language (IDL) for gRPC. The structure of the request and response messages (e.g., `PredictRequest`, `PredictResponse`) is defined in `.proto` files. Clients and servers use code generated from these files to ensure compatibility.

## 2. Communication Workflow

The following steps outline the language-agnostic process for a SentryShot plugin to communicate with a gRPC-based model server for object detection.

### Step 1: Establish gRPC Connection
The plugin must first establish a persistent gRPC connection to the model server's endpoint (e.g., `my-inference-server.local:8500`).

### Step 2: Acquire and Pre-process Image
For each video frame received from the monitor, it must be pre-processed to match the model's specific input requirements. There are two common approaches:

**A) Raw Pixel Tensor (e.g., for YOLO models)**
This is the most common method for modern object detectors.
1.  **Resize**: The frame is resized to the exact input dimensions expected by the model (e.g., 640x640).
2.  **Normalize**: Pixel values are normalized. A common technique is to scale pixel values from the `[0, 255]` integer range to the `[0.0, 1.0]` floating-point range.
3.  **Data Type Conversion**: The normalized pixel data is converted to the required floating-point type, such as 32-bit float (`DT_FLOAT`) or 16-bit half-precision float (`DT_HALF`).
4.  **Layout Conversion**: The memory layout of the pixel data is rearranged. Most models expect the data in a `NCHW` (Number of images, Channels, Height, Width) format.

**B) Encoded Image String (e.g., for some SSD MobileNet models)**
1.  **Resize**: The frame is resized.
2.  **Encode**: The resized image is encoded into a standard image format like JPEG. The raw bytes of the JPEG file are sent to the server.

### Step 3: Construct the gRPC `PredictRequest`
A `PredictRequest` message is constructed. Its structure depends on the pre-processing method used.

*   **`model_spec`**: Specifies the model name (e.g., "yolov8n").
*   **`inputs`**: A map containing the input tensor(s).

**For a Raw Pixel Tensor (YOLOv8 example):**
*   **Key**: The name of the input tensor (e.g., "images").
*   **Value**: A `TensorProto` object.
    *   `dtype`: `DT_FLOAT` or `DT_HALF`.
    *   `tensor_shape`: `[1, 3, Height, Width]` for a single NCHW image.
    *   `float_val` or `half_val`: The array containing the raw, pre-processed pixel data.

**For an Encoded Image String:**
*   **Key**: The name of the input tensor (e.g., "image_tensor").
*   **Value**: A `TensorProto` object.
    *   `dtype`: `DT_STRING`.
    *   `tensor_shape`: `[1]` for a single image.
    *   `string_val`: An array containing the raw JPEG bytes.

### Step 4: Send Request and Parse Response
1.  The `PredictRequest` is sent to the model server.
2.  The server returns a `PredictResponse` containing the model's output tensors.
3.  The plugin must parse these tensors. The structure varies significantly between models.

**YOLOv8 Output:**
*   A single output tensor (e.g., "output0") with shape `[1, 84, 8400]`. This tensor combines bounding box coordinates, class probabilities, and objectness scores for thousands of potential detections.
*   **Post-processing is required**: The plugin must iterate through the proposals, filter by a confidence score, and apply Non-Maximum Suppression (NMS) to eliminate duplicate boxes for the same object.

**SSD MobileNet Output:**
*   Multiple, separate output tensors are common (e.g., `detection_boxes`, `detection_scores`, `detection_classes`). Post-processing is simpler as NMS is often handled by the model itself.

## 3. Go Example (YOLOv8)

The code in the `@yolo/` directory provides a detailed example of communicating with a server running a YOLOv8 model. It demonstrates the more complex "Raw Pixel Tensor" workflow.

Key functions are `preprocessImage` and `decodeResponse`.

```go
// From yolo/object_detector.go

// preprocessImage resizes, normalizes, and converts image data to a DT_HALF tensor.
func preprocessImage(img image.Image) (*framework.TensorProto, error) {
	numPixels := int(inputWidth * inputHeight)
	halfVals := make([]int32, 3*numPixels)

	// Assumes img is a custom RGB24 type for direct pixel access
	rgb24Img := img.(*RGB24)

	// Iterate, normalize to [0,1], convert to half-precision float,
	// and arrange in NCHW format.
	for y := 0; y < int(inputHeight); y++ {
		for x := 0; x < int(inputWidth); x++ {
			pixelOffset := y*rgb24Img.Stride + x*3
			r := rgb24Img.Pix[pixelOffset]
			g := rgb24Img.Pix[pixelOffset+1]
			b := rgb24Img.Pix[pixelOffset+2]

			halfVals[y*int(inputWidth)+x] = int32(float32ToHalf(float32(r) / 255.0))
			halfVals[y*int(inputWidth)+x+numPixels] = int32(float32ToHalf(float32(g) / 255.0))
			halfVals[y*int(inputWidth)+x+(2*numPixels)] = int32(float32ToHalf(float32(b) / 255.0))
		}
	}

	return &framework.TensorProto{
		Dtype: framework.DataType_DT_HALF,
		TensorShape: &framework.TensorShapeProto{
			Dim: []*framework.TensorShapeProto_Dim{
				{Size: 1}, {Size: 3}, {Size: inputHeight}, {Size: inputWidth},
			},
		},
		HalfVal: halfVals,
	}, nil
}

// decodeResponse interprets the model's raw DT_HALF output tensor.
func decodeResponse(response *apis.PredictResponse) ([]BoundingBox, error) {
	output := response.Outputs["output0"]
	// ... error checking ...

	// Decode half-precision float tensor content into a standard float32 slice.
	outputData, err := decodeHalfTensor(output)
	if err != nil {
		return nil, err
	}

	var candidates []BoundingBox
	// ... loop through all proposals in outputData ...
	// 1. Find the class with the highest score for the proposal.
	// 2. If score > confidence_threshold, create a BoundingBox.
	// 3. Convert box from [center_x, center_y, width, height] to [xmin, ymin, xmax, ymax].
	// 4. Add to candidates list.

	// Filter overlapping boxes for the same object
	return performNMS(candidates, iouThreshold)
}
```

## 4. SentryShot Plugin Integration

To integrate this into a SentryShot plugin, the logic from the Go example would be adapted to fit inside the `detect` method of the plugin's `Detector` trait. The `detect` method would receive the raw frame bytes, perform the pre-processing and gRPC communication, and return the parsed `Detections`.