mod bindings;

pub use bindings::{
    c_detector_allocate, c_detector_detect, c_detector_free, c_detector_load_model, c_free_devices,
    c_list_devices, c_poke_devices, c_probe_device, edgetpu_device, CDetector,
};

extern "C" {
    // Sets verbosity of operating logs related to edge TPU.
    // Verbosity level can be set to [0-10], in which 10 is the most verbose.
    pub fn edgetpu_verbosity(verbosity: ::std::os::raw::c_int);
}
