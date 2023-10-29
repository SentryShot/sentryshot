mod bindings;

pub use bindings::{
    c_detector_allocate, c_detector_detect, c_detector_free, c_detector_load_model, CDetector,
};
