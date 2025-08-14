# 7. Model Server Integration

This document describes how to integrate SentryShot with an external model server (like TensorFlow Serving) using gRPC for object detection. This approach allows for offloading intensive computation to a dedicated server and using a wider variety of models.

## 1. Core Concepts

### Model Server
A model server is a specialized application designed to serve machine learning models over a network. It handles loading models, managing versions, and running inference efficiently. [TensorFlow Serving](https://www.tensorflow.org/tfx/guide/serving) is a popular example that exposes a gRPC endpoint for predictions.

### gRPC
gRPC is a high-performance, open-source universal RPC framework. Communication between the SentryShot plugin (client) and the model server is handled via gRPC calls.

### Protocol Buffers (Protobuf)
Protobuf is the Interface Definition Language (IDL) for gRPC. The structure of the request and response messages (e.g., `PredictRequest`, `PredictResponse`) is defined in `.proto` files. Clients and servers use code generated from these files to ensure compatibility.

## 2. Communication Workflow

The following steps outline the language-agnostic process for a SentryShot plugin to communicate with a gRPC-based model server for object detection.

### Step 1: Establish gRPC Connection
The plugin must first establish a persistent or per-request gRPC connection to the model server's endpoint (e.g., `my-tf-server.local:8500`).

### Step 2: Acquire and Pre-process Image
For each video frame received from the monitor:
1.  **Resize**: The frame must be resized to the exact input dimensions expected by the model (e.g., 300x300, 640x640).
2.  **Encode**: The resized image frame is then encoded into a standard image format. JPEG is a common choice for models that accept an encoded string as input. This is often more efficient than sending a large raw pixel tensor over the network.

### Step 3: Construct the gRPC `PredictRequest`
A `PredictRequest` message is constructed according to the TensorFlow Serving API.

*   **`model_spec`**: Specifies the model to be used.
    *   `name`: The name of the model deployed on the server (e.g., "yolov8").
    *   `signature_name`: (Optional) The specific model signature to use, typically "serving_default".
*   **`inputs`**: A map where each entry corresponds to one of the model's input tensors.
    *   **Key**: The name of the input tensor as defined in the model's signature (e.g., "image_tensor", "input_1").
    *   **Value**: A `TensorProto` object containing the pre-processed image data.
        *   `dtype`: The data type. For an encoded image, this is `DT_STRING`.
        *   `tensor_shape`: The shape of the input tensor. For a single encoded image, this is a 1-dimensional tensor of size 1, i.e., `[1]`.
        *   `string_val`: The actual data, which is an array containing the bytes of the encoded (e.g., JPEG) image.

### Step 4: Send Request and Parse Response
1.  The `PredictRequest` is sent to the model server by calling the `Predict` RPC method.
2.  The server returns a `PredictResponse` message. This response contains an `outputs` map.
3.  The plugin must parse the `TensorProto` objects from the `outputs` map. The keys of this map (e.g., "detection_boxes", "detection_scores", "detection_classes") correspond to the output tensors defined in the model's signature.
4.  The data from these tensors (bounding boxes, scores, class IDs) is extracted and converted into the SentryShot `Detection` format.

## 3. Go Example

The provided `stream-detection-grpc/main.go` file serves as an excellent practical example of this workflow. It reads from a video stream, pre-processes frames, and sends them to a TensorFlow Serving gRPC endpoint.

```go
// main.go

package main

import (
	"context"
	"flag"
	"image"
	"log"
	"sync"
	"time"

	"camera/config"
	framework "github.com/figroc/tensorflow-serving-client/v2/go/tensorflow/core/framework"
	apis "github.com/figroc/tensorflow-serving-client/v2/go/tensorflow_serving/apis"
	"gocv.io/x/gocv"
	"google.golang.org/grpc"
)

// ... main function to handle flags and setup ...

func processStream(stream config.StreamConfig, client apis.PredictionServiceClient, modelName, inputName string) {
	log.Printf("Processing stream: %s (%s)", stream.Name, stream.URL)

	// 1. Open video stream
	webcam, err := gocv.VideoCaptureFile(stream.URL)
	if err != nil {
		log.Printf("failed to open video capture for stream %s: %v", stream.Name, err)
		return
	}
	defer webcam.Close()

	img := gocv.NewMat()
	defer img.Close()

	for {
		if ok := webcam.Read(&img); !ok {
			log.Printf("cannot read frame from stream %s\n", stream.Name)
			break
		}
		if img.Empty() {
			continue
		}

		// 2. Pre-process the image: resize and encode to JPEG
		gocv.Resize(img, &img, image.Point{X: 300, Y: 300}, 0, 0, gocv.InterpolationDefault)
		imgBytes, err := gocv.IMEncode(gocv.JPEGFileExt, img)
		if err != nil {
			log.Printf("failed to encode image for stream %s: %v", stream.Name, err)
			continue
		}
		defer imgBytes.Close()

		// 3. Construct the gRPC PredictRequest
		request := &apis.PredictRequest{
			ModelSpec: &apis.ModelSpec{
				Name: modelName,
			},
			Inputs: map[string]*framework.TensorProto{
				inputName: {
					Dtype: framework.DataType_DT_STRING,
					TensorShape: &framework.TensorShapeProto{
						Dim: []*framework.TensorShapeProto_Dim{
							{Size: 1},
						},
					},
					StringVal: [][]byte{imgBytes.GetBytes()},
				},
			},
		}

		// 4. Send the request and (optionally) process the response
		// Note: This example ignores the response, but a real implementation
		// would parse the detection results from it.
		_, err = client.Predict(context.Background(), request)
		if err != nil {
			log.Printf("failed to run detection for stream %s: %v", stream.Name, err)
			continue
		}
	}
}
```

## 4. SentryShot Plugin Integration

To integrate this into a SentryShot plugin, the logic from the `processStream` function would be adapted to fit inside the `detect` method of the plugin's `Detector` trait. The `detect` method would receive the raw frame bytes, perform the pre-processing and gRPC communication, and return the parsed `Detections`.
