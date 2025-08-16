// SPDX-License-Identifier: GPL-2.0-or-later

use async_trait::async_trait;
use crate::config::Config;
use common::{ArcMsgLogger, Detections, DynError, LogLevel};
use http::uri::InvalidUri;
use std::num::NonZeroU16;
use std::sync::Arc;
use tokio::sync::OnceCell;
use tonic::transport::Channel;

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

use tensorflow::{
    serving::prediction_service_client::PredictionServiceClient,
    serving::{ModelSpec, PredictRequest, PredictResponse},
    tensor_shape_proto::Dim as TensorShapeProto_Dim,
    DataType, TensorProto, TensorShapeProto,
};

#[async_trait]
pub(crate) trait Detector: Send + Sync {
    async fn detect(&self, data: Vec<u8>) -> Result<Option<Detections>, DynError>;
    fn width(&self) -> NonZeroU16;
    fn height(&self) -> NonZeroU16;
}

pub(crate) type ArcDetector = Arc<dyn Detector>;

pub(crate) struct GrpcDetector {
    logger: ArcMsgLogger,
    client: OnceCell<PredictionServiceClient<Channel>>,
    host: String,
    model_name: String,
    input_name: String,
    output_name: String,
    width: NonZeroU16,
    height: NonZeroU16,
}

impl GrpcDetector {
    pub(crate) fn new(logger: ArcMsgLogger, config: &Config) -> Self {
        Self {
            logger,
            client: OnceCell::new(),
            host: config.host.clone(),
            model_name: config.model_name.clone(),
            input_name: config.input_tensor.clone(),
            output_name: config.output_tensor.clone(),
            width: config.input_width,
            height: config.input_height,
        }
    }

    async fn get_client(&self) -> Result<&PredictionServiceClient<Channel>, GrpcDetectorError> {
        self.client
            .get_or_try_init(|| async {
                let channel = Channel::from_shared(format!("http://{}", self.host))
                    .map_err(GrpcDetectorError::InvalidUri)?
                    .connect()
                    .await
                    .map_err(GrpcDetectorError::Connection)?;

                let client = PredictionServiceClient::new(channel);

                self.logger.log(
                    LogLevel::Info,
                    &format!("GrpcDetector: connected to {}", self.host),
                );
                Ok(client)
            })
            .await
    }
}

#[derive(Debug, thiserror::Error)]
pub(crate) enum GrpcDetectorError {
    #[error("invalid URI: {0}")]
    InvalidUri(#[from] InvalidUri),
    #[error("connection error: {0}")]
    Connection(#[from] tonic::transport::Error),
    #[error("gRPC request failed: {0}")]
    GrpcRequest(#[from] tonic::Status),
    #[error("missing output tensor: {0}")]
    MissingOutputTensor(String),
    #[error("invalid tensor data type")]
    InvalidTensorDataType,
    #[error("invalid tensor shape")]
    InvalidTensorShape,
    #[error("failed to decode YOLO response: {0}")]
    DecodeYoloResponse(String),
}

#[async_trait]
impl Detector for GrpcDetector {
    async fn detect(&self, data: Vec<u8>) -> Result<Option<Detections>, DynError> {
        self.logger
            .log(LogLevel::Debug, "GrpcDetector: received detection request.");

        let client = self.get_client().await?;

        // Convert RGB24 Vec<u8> to NCHW half-precision float tensor
        let input_tensor_data = rgb_to_nchw_half(&data, self.width.get(), self.height.get())?;

        let tensor_shape = TensorShapeProto {
            dim: vec![
                TensorShapeProto_Dim {
                    size: 1,
                    name: String::new(),
                },
                TensorShapeProto_Dim {
                    size: 3,
                    name: String::new(),
                },
                TensorShapeProto_Dim {
                    size: self.height.get() as i64,
                    name: String::new(),
                },
                TensorShapeProto_Dim {
                    size: self.width.get() as i64,
                    name: String::new(),
                },
            ],
            unknown_rank: false,
        };

        let tensor_proto = TensorProto {
            dtype: DataType::DtHalf.into(), // For YOLOv8 FP16
            tensor_shape: Some(tensor_shape),
            half_val: input_tensor_data,
            ..Default::default()
        };

        let mut inputs = std::collections::HashMap::new();
        inputs.insert(self.input_name.clone(), tensor_proto);

        let predict_request = PredictRequest {
            model_spec: Some(ModelSpec {
                name: self.model_name.clone(),
                version: None,
                signature_name: String::new(),
            }),
            inputs,
            output_filter: vec![self.output_name.clone()],
        };

        let response = client
            .clone()
            .predict(predict_request)
            .await
            .map_err(|e| Box::new(GrpcDetectorError::GrpcRequest(e)) as DynError)?
            .into_inner();

        let detections = decode_yolo_response(
            response,
            &self.output_name,
            self.width.get(),
            self.height.get(),
            &self.logger,
        )?;

        Ok(Some(detections))
    }

    fn width(&self) -> NonZeroU16 {
        self.width
    }

    fn height(&self) -> NonZeroU16 {
        self.height
    }
}

// Helper function to convert RGB24 Vec<u8> to NCHW half-precision float Vec<i32>
// The input `data` is expected to be a flat RGB24 byte array (width * height * 3 bytes).
// Output is a flat NCHW half-precision float array (3 * width * height half-floats).
fn rgb_to_nchw_half(
    data: &[u8],
    width: u16,
    height: u16,
) -> Result<Vec<i32>, GrpcDetectorError> {
    let num_pixels = (width as usize) * (height as usize);
    let mut half_vals = vec![0i32; 3 * num_pixels]; // 3 channels * num_pixels

    if data.len() != num_pixels * 3 {
        return Err(GrpcDetectorError::InvalidTensorShape);
    }

    for y in 0..height as usize {
        for x in 0..width as usize {
            let pixel_offset = (y * width as usize + x) * 3;
            let r = data[pixel_offset];
            let g = data[pixel_offset + 1];
            let b = data[pixel_offset + 2];

            // Normalize to [0, 1] and convert to half-precision float
            // Store in NCHW format
            half_vals[0 * num_pixels + y * width as usize + x] = 
                float32_to_f16(r as f32 / 255.0) as i32;
            half_vals[1 * num_pixels + y * width as usize + x] = 
                float32_to_f16(g as f32 / 255.0) as i32;
            half_vals[2 * num_pixels + y * width as usize + x] = 
                float32_to_f16(b as f32 / 255.0) as i32;
        }
    }
    Ok(half_vals)
}

// float32_to_f16 converts a 32-bit float to a 16-bit half-precision float.
fn float32_to_f16(val: f32) -> u16 {
    let bits = val.to_bits();
    let sign = (bits >> 16) & 0x8000;
    let mut exp = ((bits >> 23) & 0xff) as i32 - 127;
    let mant = bits & 0x7fffff;

    if exp > 15 { // Exponent overflow -> infinity
        return (sign | 0x7c00) as u16;
    }
    if exp < -14 { // Exponent underflow -> flush to zero
        return sign as u16;
    }
    exp += 15;
    let mant = mant >> 13;
    (sign | (exp as u32) << 10 | mant) as u16
}

// Placeholder for decoding YOLO response and performing NMS
fn decode_yolo_response(
    response: PredictResponse,
    output_name: &str,
    _width: u16,
    _height: u16,
    logger: &ArcMsgLogger,
) -> Result<Detections, GrpcDetectorError> {
    logger.log(
        LogLevel::Debug,
        &format!("GrpcDetector: decoding YOLO response for output '{}'", output_name),
    );

    let output_tensor = response
        .outputs
        .get(output_name)
        .ok_or_else(|| GrpcDetectorError::MissingOutputTensor(output_name.to_string()))?;

    // Assuming YOLOv8 output format: [1, 84, 8400]
    // 84: 4 (bbox) + 80 (classes)
    // 8400: number of proposals
    let shape = output_tensor
        .tensor_shape
        .as_ref()
        .ok_or(GrpcDetectorError::InvalidTensorShape)?;

    if shape.dim.len() != 3
        || shape.dim[0].size != 1
        || shape.dim[1].size != 84
        || shape.dim[2].size != 8400
    {
        return Err(GrpcDetectorError::InvalidTensorShape);
    }

    // The output tensor contains float values.
    let _raw_output_data = &output_tensor.float_val; // Assuming float_val for output

    // TODO: Implement actual YOLOv8 post-processing (confidence filtering, NMS, etc.)
    // This is a complex step that involves:
    // 1. Iterating through `raw_output_data` to extract bounding boxes, confidence scores, and class probabilities.
    // 2. Applying a confidence threshold.
    // 3. Converting bounding box coordinates (e.g., from center_x, center_y, width, height to xmin, ymin, xmax, ymax).
    // 4. Performing Non-Maximum Suppression (NMS) to remove overlapping boxes.
    // 5. Mapping class IDs to actual labels (requires a label map, not provided in this context).

    logger.log(
        LogLevel::Info,
        "YOLOv8 post-processing (confidence filtering, NMS) is not yet implemented.",
    );

    Ok(Vec::new()) // Return empty Detections for now
}

pub(crate) struct DetectorManager {
    detector: ArcDetector,
}

impl DetectorManager {
    pub(crate) fn new(logger: ArcMsgLogger, config: &Config) -> Self {
        Self {
            detector: Arc::new(GrpcDetector::new(logger, config)),
        }
    }

    pub(crate) fn get_detector(&self) -> ArcDetector {
        self.detector.clone()
    }
}
