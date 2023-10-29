use std::{ffi::CString, fmt::Debug, path::Path, slice::from_raw_parts};
use tflite_sys::{
    c_detector_allocate, c_detector_detect, c_detector_free, c_detector_load_model, CDetector,
};
use thiserror::Error;

#[derive(Debug, Error)]
pub enum NewDetectorError {
    #[error("detector is null")]
    DetectorNull,

    #[error("create from file")]
    CreateFromFile,

    #[error("interpreter create")]
    InterpreterCreate,

    #[error("input tensor count")]
    InputTensorCount,

    #[error("input tensor type")]
    InputTensorType,

    #[error("output tensor count")]
    OutputTensorCount,

    #[error("load model: {0}")]
    LoadModel(i32),
}

const ERROR_CREATE_FROM_FILE: i32 = 10000;
const ERROR_INTERPRETER_CREATE: i32 = 10001;
const ERROR_INPUT_TENSOR_COUNT: i32 = 10002;
const ERROR_INPUT_TENSOR_TYPE: i32 = 10003;
const ERROR_OUTPUT_TENSOR_COUNT: i32 = 10004;

#[derive(Debug, Error)]
pub enum DetectError {
    #[error("buffer size: {0}vs{1}")]
    BufferSize(usize, usize),

    #[error("output tensor type")]
    OutputTensorType,

    #[error("detect: {0}")]
    Detect(i32),

    #[error("parse output tensors: {0:?} {1}")]
    ParseOutputTensors([usize; 4], ParseOutputTensorsError),
}

const ERROR_OUTPUT_TENSOR_TYPE: i32 = 20000;

pub struct Detector {
    c_detector: *mut CDetector,
    input_tensor_size: usize,
}

unsafe impl Send for Detector {}

impl Detector {
    pub fn new(model_path: &Path) -> Result<Self, NewDetectorError> {
        use NewDetectorError::*;
        let model_path = model_path.to_str().unwrap();
        let model_path = CString::new(model_path).unwrap();
        unsafe {
            let c_detector = c_detector_allocate();
            if c_detector.is_null() {
                return Err(DetectorNull);
            }

            let mut input_tensor_size = 0;
            let res =
                c_detector_load_model(c_detector, model_path.as_ptr(), &mut input_tensor_size);
            if res != 0 {
                return Err(match res {
                    ERROR_CREATE_FROM_FILE => CreateFromFile,
                    ERROR_INTERPRETER_CREATE => InterpreterCreate,
                    ERROR_INPUT_TENSOR_COUNT => InputTensorCount,
                    ERROR_INPUT_TENSOR_TYPE => InputTensorCount,
                    ERROR_OUTPUT_TENSOR_COUNT => OutputTensorCount,
                    _ => LoadModel(res),
                });
            }

            Ok(Self {
                c_detector,
                input_tensor_size,
            })
        }
    }

    pub fn detect(&mut self, buf: &[u8]) -> Result<Vec<Detection>, DetectError> {
        use DetectError::*;
        assert_eq!(self.input_tensor_size, buf.len());
        if self.input_tensor_size != buf.len() {
            return Err(BufferSize(self.input_tensor_size, buf.len()));
        }
        unsafe {
            let t0_data: *mut *mut u8 = &mut std::ptr::null_mut();
            let t1_data: *mut *mut u8 = &mut std::ptr::null_mut();
            let t2_data: *mut *mut u8 = &mut std::ptr::null_mut();
            let t3_data: *mut *mut u8 = &mut std::ptr::null_mut();
            let mut t0_size = 0;
            let mut t1_size = 0;
            let mut t2_size = 0;
            let mut t3_size = 0;

            let res = c_detector_detect(
                self.c_detector,
                buf.as_ptr(),
                buf.len(),
                t0_data,
                t1_data,
                t2_data,
                t3_data,
                &mut t0_size,
                &mut t1_size,
                &mut t2_size,
                &mut t3_size,
            );
            if res != 0 {
                return Err(match res {
                    ERROR_OUTPUT_TENSOR_TYPE => OutputTensorType,
                    _ => Detect(res),
                });
            }

            let t0 = from_raw_parts(*t0_data, t0_size);
            let t1 = from_raw_parts(*t1_data, t1_size);
            let t2 = from_raw_parts(*t2_data, t2_size);
            let t3 = from_raw_parts(*t3_data, t3_size);

            parse_output_tensors(t0, t1, t2, t3)
                .map_err(|e| ParseOutputTensors([t0_size, t1_size, t2_size, t3_size], e))
        }
    }
}

impl Drop for Detector {
    fn drop(&mut self) {
        unsafe { c_detector_free(self.c_detector) }
    }
}

#[derive(Debug, Error)]
pub enum ParseOutputTensorsError {
    #[error("count tensor is empty")]
    GetCount,

    #[error("score tensor out of bounds: {0}")]
    ScoreBounds(usize),

    #[error("class tensor out of bounds: {0}")]
    ClassBounds(usize),

    #[error("class out of range 0-255: {0}")]
    ClassRange(f32),

    #[error("cordinate out of bounds: {0}")]
    RectBounds(usize),
}

fn parse_output_tensors(
    t0: &[u8],
    t1: &[u8],
    t2: &[u8],
    t3: &[u8],
) -> Result<Vec<Detection>, ParseOutputTensorsError> {
    use ParseOutputTensorsError::*;
    let t0 = u8_to_f32(t0);
    let t1 = u8_to_f32(t1);
    let t2 = u8_to_f32(t2);
    let t3 = u8_to_f32(t3);

    let mut detections = Vec::new();
    let count = *t3.first().ok_or(GetCount)? as usize;
    for i in 0..count {
        let score = *t2.get(i).ok_or(ScoreBounds(i))?;
        let class = *t1.get(i).ok_or(ClassBounds(i))?;
        if !(-0.0..=255.0).contains(&class) {
            return Err(ClassRange(class));
        }

        let class = class as u8;
        let top = t0.get(4 * i).ok_or(RectBounds(i))?;
        let left = t0.get(4 * i + 1).ok_or(RectBounds(i))?;
        let bottom = t0.get(4 * i + 2).ok_or(RectBounds(i))?;
        let right = t0.get(4 * i + 3).ok_or(RectBounds(i))?;

        let top = top.max(0.0).min(1.0);
        let left = left.max(0.0).min(1.0);
        let bottom = bottom.max(0.0).min(1.0);
        let right = right.max(0.0).min(1.0);

        detections.push(Detection {
            score,
            class,
            top,
            left,
            bottom,
            right,
        });
    }
    Ok(detections)
}

fn u8_to_f32(input: &[u8]) -> Vec<f32> {
    input
        .chunks_exact(4)
        .map(|i| f32::from_ne_bytes([i[0], i[1], i[2], i[3]]))
        .collect()
}

pub struct Detection {
    pub score: f32,
    pub class: u8,
    pub top: f32,
    pub left: f32,
    pub bottom: f32,
    pub right: f32,
}

impl Debug for Detection {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(
            f,
            "score={:.2} class={} area=[{:.2}, {:.2}, {:.2}, {:.2}]",
            self.score, self.class, self.top, self.left, self.bottom, self.right,
        )
    }
}
