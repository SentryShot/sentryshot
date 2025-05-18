mod bindings;

pub use bindings::{
    CDetector, TfLiteQuantizationParams, TfLiteTensor, TfLiteTensorByteSize,
    TfLiteTensorCopyFromBuffer, TfLiteTensorData, TfLiteTensorDim, TfLiteTensorNumDims,
    TfLiteTensorQuantizationParams, TfLiteTensorType, c_detector_allocate, c_detector_free,
    c_detector_input_tensor, c_detector_input_tensor_count, c_detector_invoke_interpreter,
    c_detector_load_model, c_detector_output_tensor, c_detector_output_tensor_count,
    c_free_devices, c_list_devices, c_poke_devices, c_probe_device, edgetpu_device,
};

unsafe extern "C" {
    // Sets verbosity of operating logs related to edge TPU.
    // Verbosity level can be set to [0-10], in which 10 is the most verbose.
    pub fn edgetpu_verbosity(verbosity: ::std::os::raw::c_int);
}
