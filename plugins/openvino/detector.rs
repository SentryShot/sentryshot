// SPDX-License-Identifier: GPL-2.0-or-later

use async_trait::async_trait;
use common::{ArcMsgLogger, Detections, DynError, LogLevel};
use std::num::NonZeroU16;
use std::sync::Arc;

mod tensorflow {
    mod error {
        include!(concat!(env!("OUT_DIR"), "/tensorflow.error.rs"));
    }
    mod grpc {
        include!(concat!(env!("OUT_DIR"), "/tensorflow.grpc.rs"));
    }
    include!(concat!(env!("OUT_DIR"), "/tensorflow.rs"));
    pub mod serving {
        include!(concat!(env!("OUT_DIR"), "/tensorflow.serving.rs"));
    }
}

// Assuming these paths based on common tonic/prost generation and proto file structure
// The actual paths might vary slightly depending on the tonic-prost-build configuration.
use tensorflow::{TensorProto, DataType, TensorShapeProto};
use tensorflow::tensor_shape_proto::Dim as TensorShapeProto_Dim;
use tensorflow::serving::{PredictRequest, ModelSpec};


#[async_trait]
pub(crate) trait Detector: Send + Sync {
    async fn detect(&self, data: Vec<u8>) -> Result<Option<Detections>, DynError>;
    fn width(&self) -> NonZeroU16;
    fn height(&self) -> NonZeroU16;
}

pub(crate) type ArcDetector = Arc<dyn Detector>;

pub(crate) struct DummyDetector {
    logger: ArcMsgLogger,
    width: NonZeroU16,
    height: NonZeroU16,
}

impl DummyDetector {
    pub(crate) fn new(logger: ArcMsgLogger) -> Self {
        Self {
            logger,
            width: NonZeroU16::new(300).unwrap(),
            height: NonZeroU16::new(300).unwrap(),
        }
    }
}

#[async_trait]
impl Detector for DummyDetector {
    async fn detect(&self, data: Vec<u8>) -> Result<Option<Detections>, DynError> {
        self.logger
            .log(LogLevel::Info, "DummyDetector: received detection request.");

        // Placeholder for constructing PredictRequest
        let model_spec = ModelSpec {
            name: "my_model".to_string(), // Replace with actual model name
            version: None, // Optional: specify model version
            signature_name: "".to_string(), // Optional: specify signature name
        };

        // Assuming the input data is a flat Vec<u8> representing image pixels
        // This conversion is highly dependent on the model's input requirements.
        // For a dummy, we'll just put the bytes directly into a float_val for now,
        // which is likely incorrect for a real scenario but demonstrates the structure.
        // A real implementation would involve image processing (e.g., resizing, normalization)
        // and converting to the correct tensor data type (e.g., f32).
        let tensor_shape = TensorShapeProto {
            dim: vec![
                // Example dimensions: batch_size, height, width, channels
                TensorShapeProto_Dim { size: 1, name: "".to_string() },
                TensorShapeProto_Dim { size: self.height.get() as i64, name: "".to_string() },
                TensorShapeProto_Dim { size: self.width.get() as i64, name: "".to_string() },
                TensorShapeProto_Dim { size: 3, name: "".to_string() }, // Assuming 3 channels (RGB)
            ],
            unknown_rank: false,
        };

        let tensor_proto = TensorProto {
            dtype: DataType::DtFloat.into(), // Assuming float data type
            tensor_shape: Some(tensor_shape),
            // For a real image, you'd convert `data` into a flat list of floats
            // and put it into `float_val`. Here, we're just using a placeholder.
            float_val: data.iter().map(|&x| x as f32).collect(), // This is a very naive conversion
            ..Default::default() // Fill in other fields with default values
        };

        let mut inputs = std::collections::HashMap::new();
        inputs.insert("input_tensor".to_string(), tensor_proto); // Replace "input_tensor" with actual input tensor name

        let predict_request = PredictRequest {
            model_spec: Some(model_spec),
            inputs,
            output_filter: vec![], // Specify outputs if needed
        };

        // In a real scenario, you would now send this predict_request to a TensorFlow Serving model server
        // using a gRPC client (e.g., tonic client).
        self.logger
            .log(LogLevel::Info, &format!("DummyDetector: Constructed PredictRequest: {:?}", predict_request.model_spec));

        Ok(Some(Vec::new()))
    }

    fn width(&self) -> NonZeroU16 {
        self.width
    }

    fn height(&self) -> NonZeroU16 {
        self.height
    }
}

pub(crate) struct DetectorManager {
    detector: ArcDetector,
}

impl DetectorManager {
    pub(crate) fn new(logger: ArcMsgLogger) -> Self {
        Self {
            detector: Arc::new(DummyDetector::new(logger)),
        }
    }

    pub(crate) fn get_detector(&self) -> ArcDetector {
        self.detector.clone()
    }
}
