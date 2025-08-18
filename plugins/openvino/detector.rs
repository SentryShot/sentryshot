// SPDX-License-Identifier: GPL-2.0-or-later

use async_trait::async_trait;
use crate::config::Config;
use common::{ArcMsgLogger, Detection, Detections, DynError, LogLevel, RectangleNormalized, Region};
use http::uri::InvalidUri;
use std::num::{NonZeroU16, NonZeroU32};
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
    #[error("tensor data not found")]
    TensorDataNotFound,
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

fn half_to_float32(h: u16) -> f32 {
    let sign = (h & 0x8000) as u32;
    let exp = (h & 0x7C00) >> 10;
    let mant = (h & 0x03FF) as u32;

    let bits = if exp == 0x1F {
        0x7F800000 | (mant << 13)
    } else if exp == 0 {
        if mant != 0 {
            let mut exp_val = 1 - 15;
            let mut mant_val = mant;
            while (mant_val & 0x0400) == 0 {
                mant_val <<= 1;
                exp_val -= 1;
            }
            let mant32 = (mant_val & 0x03FF) << 13;
            ((exp_val + 127) as u32) << 23 | mant32
        } else {
            0
        }
    } else {
        ((exp - 15 + 127) as u32) << 23 | (mant << 13)
    };

    f32::from_bits(sign << 16 | bits)
}

fn decode_half_tensor(tensor: &TensorProto) -> Result<Vec<f32>, GrpcDetectorError> {
    if tensor.dtype != DataType::DtHalf as i32 {
        return Err(GrpcDetectorError::InvalidTensorDataType);
    }
    if !tensor.tensor_content.is_empty() {
        Ok(tensor
            .tensor_content
            .chunks_exact(2)
            .map(|chunk| half_to_float32(u16::from_le_bytes([chunk[0], chunk[1]])))
            .collect())
    } else if !tensor.half_val.is_empty() {
        Ok(tensor
            .half_val
            .iter()
            .map(|&h| half_to_float32(h as u16))
            .collect())
    } else {
        Err(GrpcDetectorError::TensorDataNotFound)
    }
}

struct BoundingBox {
    xmin: f32,
    ymin: f32,
    xmax: f32,
    ymax: f32,
    confidence: f32,
    class_id: usize,
}

fn decode_yolo_response(
    response: PredictResponse,
    output_name: &str,
    _width: u16,
    _height: u16,
    logger: &ArcMsgLogger,
) -> Result<Detections, GrpcDetectorError> {
    const CONFIDENCE_THRESHOLD: f32 = 0.5;
    const NMS_IOU_THRESHOLD: f32 = 0.45;
    const NUM_CLASSES: usize = 80;

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
    {
        return Err(GrpcDetectorError::InvalidTensorShape);
    }

    let num_proposals = shape.dim[2].size as usize;
    let raw_data = decode_half_tensor(output_tensor)?;

    let mut candidates: Vec<BoundingBox> = Vec::new();
    for i in 0..num_proposals {
        let cx = raw_data[0 * num_proposals + i];
        let cy = raw_data[1 * num_proposals + i];
        let w = raw_data[2 * num_proposals + i];
        let h = raw_data[3 * num_proposals + i];

        let mut max_class_score = 0.0;
        let mut max_class_id = 0;
        for j in 0..NUM_CLASSES {
            let class_score = raw_data[(4 + j) * num_proposals + i];
            if class_score > max_class_score {
                max_class_score = class_score;
                max_class_id = j;
            }
        }

        if max_class_score > CONFIDENCE_THRESHOLD {
            candidates.push(BoundingBox {
                xmin: cx - w * 0.5,
                ymin: cy - h * 0.5,
                xmax: cx + w * 0.5,
                ymax: cy + h * 0.5,
                confidence: max_class_score,
                class_id: max_class_id,
            });
        }
    }

    let final_boxes = perform_nms(candidates, NMS_IOU_THRESHOLD);

    let detections = final_boxes
        .into_iter()
        .map(|bbox| {
            logger.log(LogLevel::Info, &format!("Detected class ID: {}", bbox.class_id));
            Detection {
                label: format!("class{}", bbox.class_id).try_into().unwrap(),
                score: bbox.confidence,
                region: Region {
                    rectangle: parse_rect(bbox.ymin, bbox.xmin, bbox.ymax, bbox.xmax),
                    polygon: None,
                },
            }
        })
        .collect();

    Ok(detections)
}

fn parse_rect(top: f32, left: f32, bottom: f32, right: f32) -> Option<RectangleNormalized> {
    #[allow(
        clippy::cast_sign_loss,
        clippy::cast_possible_truncation,
        clippy::as_conversions
    )]
    fn scale(v: f32) -> u32 {
        (v * 1_000_000.0) as u32
    }
    let top = scale(top);
    let left = scale(left);
    let bottom = scale(bottom);
    let right = scale(right);
    if top > bottom || left > right {
        return None;
    }
    Some(RectangleNormalized {
        x: left,
        y: top,
        width: NonZeroU32::new(right - left)?,
        height: NonZeroU32::new(bottom - top)?,
    })
}

fn perform_nms(mut boxes: Vec<BoundingBox>, iou_threshold: f32) -> Vec<BoundingBox> {
    if boxes.is_empty() {
        return Vec::new();
    }

    boxes.sort_by(|a, b| b.confidence.partial_cmp(&a.confidence).unwrap());

    let mut final_boxes = Vec::new();
    while !boxes.is_empty() {
        let best_box = boxes.remove(0);
        final_boxes.push(best_box);
        boxes.retain(|b| calculate_iou(&final_boxes.last().unwrap(), b) < iou_threshold);
    }
    final_boxes
}

fn calculate_iou(box1: &BoundingBox, box2: &BoundingBox) -> f32 {
    let x1 = box1.xmin.max(box2.xmin);
    let y1 = box1.ymin.max(box2.ymin);
    let x2 = box1.xmax.min(box2.xmax);
    let y2 = box1.ymax.min(box2.ymax);

    let intersection = (x2 - x1).max(0.0) * (y2 - y1).max(0.0);
    let area1 = (box1.xmax - box1.xmin) * (box1.ymax - box1.ymin);
    let area2 = (box2.xmax - box2.xmin) * (box2.ymax - box2.ymin);
    let union = area1 + area2 - intersection;

    if union > 0.0 {
        intersection / union
    } else {
        0.0
    }
}

pub(crate) struct DetectorManager {
    detector: ArcDetector,
    detector_runtime: Option<tokio::runtime::Runtime>,
}

impl DetectorManager {
    pub(crate) fn new(logger: ArcMsgLogger, config: &Config) -> Self {
        let detector_runtime = tokio::runtime::Runtime::new().expect("failed to create detector runtime");
        Self {
            detector: Arc::new(GrpcDetector::new(logger, config)),
            detector_runtime: Some(detector_runtime),
        }
    }

    pub(crate) fn get_detector(&self) -> ArcDetector {
        self.detector.clone()
    }

    pub(crate) async fn detect(&self, data: Vec<u8>) -> Result<Option<Detections>, DynError> {
        let detector = self.detector.clone();
        self.detector_runtime
            .as_ref().expect("runtime not set")
            .handle()
            .spawn(async move {
                detector.detect(data).await
            })
        .await? // Await the spawned task
    }
}

impl Drop for DetectorManager {
    fn drop(&mut self) {
        // This method is called when the DetectorManager instance is dropped.
        // We shut down the associated Tokio runtime in the background.
        self.detector_runtime.take().map(|rt| rt.shutdown_background());
    }
}
