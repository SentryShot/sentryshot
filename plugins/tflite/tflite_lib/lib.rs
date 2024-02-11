use std::{
    ffi::{c_uint, CStr, CString, NulError},
    fmt::{Debug, Display, Formatter},
    os::raw::c_int,
    path::{Path, PathBuf},
    process::{Command, Stdio},
    slice::{self, from_raw_parts},
    str::FromStr,
    time::Duration,
};
use tflite_sys::{
    c_detector_allocate, c_detector_detect, c_detector_free, c_detector_load_model, c_free_devices,
    c_list_devices, c_poke_devices, c_probe_device, CDetector,
};
use thiserror::Error;

#[derive(Debug, Error)]
pub enum NewDetectorError {
    #[error("detector is null")]
    DetectorNull,

    #[error("path to string: {0}")]
    PathToString(PathBuf),

    #[error("convert to CString: {0}")]
    ConvertToCString(#[from] NulError),

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

    #[error("create edgetpu delegate")]
    EdgetpuDelegateCreate,

    #[error("load model: {0}")]
    LoadModel(i32),

    #[error("probe device: {0}, '{1}'")]
    ProbeDevice(ProbeDeviceError, String),

    #[error("debug device: {0}")]
    DebugDevice(#[from] DebugDeviceError),
}

const ERROR_CREATE_FROM_FILE: c_int = 10000;
const ERROR_INTERPRETER_CREATE: c_int = 10001;
const ERROR_INPUT_TENSOR_COUNT: c_int = 10002;
const ERROR_INPUT_TENSOR_TYPE: c_int = 10003;
const ERROR_OUTPUT_TENSOR_COUNT: c_int = 10004;
const ERROR_EDGETPU_DELEGATE_CREATE: c_int = 10005;

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
    pub fn new(
        model_path: &Path,
        edgetpu: Option<&EdgetpuDevice>,
    ) -> Result<Self, NewDetectorError> {
        use NewDetectorError::*;
        let model_path = model_path
            .to_str()
            .ok_or_else(|| NewDetectorError::PathToString(model_path.to_path_buf()))?;
        let model_path = CString::new(model_path)?;

        unsafe {
            let c_detector = c_detector_allocate();
            if c_detector.is_null() {
                return Err(DetectorNull);
            }

            let mut input_tensor_size = 0;
            let res = match edgetpu {
                Some(device) => {
                    if let Err(e) = probe_device(&device.path) {
                        return Err(ProbeDevice(e, device.path.to_owned()));
                    };
                    let path = CString::new(device.path.to_owned())?;
                    c_detector_load_model(
                        c_detector,
                        model_path.as_ptr(),
                        &mut input_tensor_size,
                        path.as_ptr(),
                        device.typ.as_uint(),
                    )
                }
                None => c_detector_load_model(
                    c_detector,
                    model_path.as_ptr(),
                    &mut input_tensor_size,
                    std::ptr::null(),
                    0,
                ),
            };
            if res != 0 {
                return Err(match res {
                    ERROR_CREATE_FROM_FILE => CreateFromFile,
                    ERROR_INTERPRETER_CREATE => InterpreterCreate,
                    ERROR_INPUT_TENSOR_COUNT => InputTensorCount,
                    ERROR_INPUT_TENSOR_TYPE => InputTensorCount,
                    ERROR_OUTPUT_TENSOR_COUNT => OutputTensorCount,
                    ERROR_EDGETPU_DELEGATE_CREATE => EdgetpuDelegateCreate,
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
    fn fmt(&self, f: &mut Formatter<'_>) -> std::fmt::Result {
        write!(
            f,
            "score={:.2} class={} area=[{:.2}, {:.2}, {:.2}, {:.2}]",
            self.score, self.class, self.top, self.left, self.bottom, self.right,
        )
    }
}

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum EdgetpuDeviceType {
    Pci,
    Usb,
}

impl EdgetpuDeviceType {
    fn as_uint(&self) -> c_uint {
        match self {
            EdgetpuDeviceType::Pci => 0,
            EdgetpuDeviceType::Usb => 1,
        }
    }
}

impl Display for EdgetpuDeviceType {
    fn fmt(&self, f: &mut Formatter<'_>) -> std::fmt::Result {
        match self {
            EdgetpuDeviceType::Pci => write!(f, "PCI"),
            EdgetpuDeviceType::Usb => write!(f, "USB"),
        }
    }
}

pub struct EdgetpuDevice {
    pub typ: EdgetpuDeviceType,
    pub path: String,
}

impl Display for EdgetpuDevice {
    fn fmt(&self, f: &mut Formatter<'_>) -> std::fmt::Result {
        write!(f, "{}: {}", self.typ, self.path)
    }
}

#[derive(Debug, Error)]
#[error("unknown edgetpu device type '{0}', expected 'usb' or 'pci'")]
pub struct UnknownEdgetpuDeviceType(String);

impl FromStr for EdgetpuDeviceType {
    type Err = UnknownEdgetpuDeviceType;

    fn from_str(s: &str) -> Result<Self, Self::Err> {
        match s.to_lowercase().as_str() {
            "usb" => Ok(Self::Usb),
            "pci" => Ok(Self::Pci),
            _ => Err(UnknownEdgetpuDeviceType(s.to_owned())),
        }
    }
}

#[derive(Debug, Error)]
pub enum DebugDeviceError {
    #[error("failed to find device: '{0}'")]
    DeviceNotFound(String),

    #[error("device exists but something went wrong: '{0}'")]
    Exists(String),
}

pub fn debug_device(path: String, devices: &[EdgetpuDevice]) -> DebugDeviceError {
    print_device(&path, devices);
    use DebugDeviceError::*;
    if !Path::new(&path).exists() {
        return DeviceNotFound(path);
    }
    Exists(path)
}

fn print_device(path: &str, devices: &[EdgetpuDevice]) {
    println!("Found {} edgetpu devices", devices.len());
    for device in devices {
        println!("{}", device)
    }
    let Some(parent) = Path::new(path).parent() else {
        println!("device path does not have a parent: {:?}", path);
        return;
    };
    let parent = parent.to_str().unwrap_or("");
    println!("ls -la {}", parent);
    let result = Command::new("ls")
        .arg("-la")
        .arg(parent)
        .stdout(Stdio::inherit())
        .stderr(Stdio::inherit())
        .spawn();
    if let Err(e) = result {
        println!("{}", e)
    }
    std::thread::sleep(Duration::from_millis(100));
}

pub fn list_edgetpu_devices() -> Vec<EdgetpuDevice> {
    poke_devices();

    let mut devices = Vec::new();
    unsafe {
        let mut num_devices = 0;
        let devices_ptr = c_list_devices(&mut num_devices);
        for device in slice::from_raw_parts(devices_ptr, num_devices) {
            let typ = match device.type_ {
                0 => EdgetpuDeviceType::Pci,
                1 => EdgetpuDeviceType::Usb,
                _ => panic!(
                    "libedgetpu returned a unknown device type: {}",
                    device.type_
                ),
            };

            let path = CStr::from_ptr(device.path);
            let path = match path.to_str() {
                Ok(v) => v.to_owned(),
                Err(_) => panic!(
                    "libedgetpu returned a device path that isn't a valid string: {:?}",
                    path
                ),
            };
            devices.push(EdgetpuDevice { typ, path })
        }
        c_free_devices(devices_ptr);
    }
    devices
}

// Sets verbosity of operating logs related to edge TPU.
// Verbosity level can be set to [0-10], in which 10 is the most verbose.
pub fn edgetpu_verbosity(verbosity: u8) {
    unsafe { tflite_sys::edgetpu_verbosity(verbosity.into()) }
}

#[derive(Debug, Error)]
pub enum ProbeDeviceError {
    #[error("failed to parse path")]
    ParsePath,

    #[error("init libusb: {0}")]
    InitLibUsb(LibUsbError),

    #[error("get device list: {0}")]
    GetDeviceList(LibUsbError),

    #[error("get port numbers: {0}")]
    GetPortNumbers(LibUsbError),

    #[error("open: {0}")]
    OpenDevice(LibUsbError),

    #[error("device not found")]
    NotFound,
}

const ERROR_USB_INIT: c_int = 20000;
const ERROR_USB_GET_DEVICE_LIST: c_int = 20001;
const ERROR_USB_GET_PORT_NUMBERS: c_int = 20002;
const ERROR_USB_OPEN_DEVICE: c_int = 20003;
const ERROR_USB_NOT_FOUND: c_int = 20004;

fn probe_device(path: &str) -> Result<(), ProbeDeviceError> {
    use ProbeDeviceError::*;
    let device_path = match DevicePath::new(path) {
        Some(v) => v,
        None => return Err(ParsePath),
    };

    unsafe {
        let mut err2: c_int = 0;
        let err = c_probe_device(
            &mut err2,
            device_path.bus_number.into(),
            device_path
                .port_numbers
                .len()
                .try_into()
                .expect("length to fit cint"),
            device_path.port_numbers.as_ptr(),
        );
        if err == 0 {
            return Ok(());
        }
        Err(match err {
            ERROR_USB_INIT => InitLibUsb(err.into()),
            ERROR_USB_GET_DEVICE_LIST => GetDeviceList(err.into()),
            ERROR_USB_GET_PORT_NUMBERS => GetPortNumbers(err.into()),
            ERROR_USB_OPEN_DEVICE => OpenDevice(err.into()),
            ERROR_USB_NOT_FOUND => NotFound,
            _ => panic!("unexpected error code: {0}", err),
        })
    }
}

#[derive(Debug, Error)]
pub enum LibUsbError {
    #[error("input/output error")]
    Io,

    #[error("invalid parameter")]
    InvaidParam,

    #[error("access denied (insufficient permissions)")]
    Access,

    #[error("no such device (it may have been disconnected)")]
    NoDevice,

    #[error("entity not found")]
    NotFound,

    #[error("resource busy")]
    Busy,

    #[error("operation timed out")]
    Timeout,

    #[error("overflow")]
    Overflow,

    #[error("pipe error")]
    Pipe,

    #[error("system call interrupted (perhaps due to signal)")]
    Interrupted,

    #[error("insufficient memory")]
    NoMem,

    #[error("operation not supported or unimplemented on this platform")]
    NotSupported,

    #[error("unknown error: {0}")]
    Unknown(c_int),
}

impl From<c_int> for LibUsbError {
    fn from(err: c_int) -> Self {
        match err {
            -1 => Self::Io,
            -2 => Self::InvaidParam,
            -3 => Self::Access,
            -4 => Self::NoDevice,
            -5 => Self::NotFound,
            -6 => Self::Busy,
            -7 => Self::Timeout,
            -8 => Self::Overflow,
            -9 => Self::Pipe,
            -10 => Self::Interrupted,
            -11 => Self::NoMem,
            -12 => Self::NotSupported,
            _ => Self::Unknown(err),
        }
    }
}

#[derive(Debug, PartialEq, Eq)]
struct DevicePath {
    bus_number: u8,
    port_numbers: Vec<u8>,
}

// Max depth for USB 3 is 7.
const MAX_USB_PATH_DEPTH: usize = 7;
const USB_PATH_PREFIX: &str = "/sys/bus/usb/devices/";

impl DevicePath {
    fn new(path: &str) -> Option<Self> {
        let path = path.strip_prefix(USB_PATH_PREFIX)?;
        let mut parts = path.split('-');
        let bus_number = parts.next()?.parse().ok()?;
        let port_numbers: Vec<u8> = parts
            .next()?
            .split('.')
            .map(|v| v.parse::<u8>())
            .collect::<Result<_, _>>()
            .ok()?;

        if parts.count() != 0 || port_numbers.is_empty() || port_numbers.len() > MAX_USB_PATH_DEPTH
        {
            return None;
        }

        Some(Self {
            bus_number,
            port_numbers,
        })
    }
}

fn poke_devices() {
    unsafe { c_poke_devices() }
}

#[cfg(test)]
mod tests {
    use super::*;
    use test_case::test_case;

    #[test_case("", None; "empty")]
    #[test_case("/sys/bus/usb/devices", None; "empty2")]
    #[test_case("/sys/bus/usb/devices/1", None; "bus_only")]
    #[test_case("/sys/bus/usb/devices/1-", None; "bus_only2")]
    #[test_case(
        "/sys/bus/usb/devices/1-1",
        Some(DevicePath { bus_number: 1, port_numbers: vec![1] });
        "1 port"
    )]
    #[test_case(
        "/sys/bus/usb/devices/1-1.2",
        Some(DevicePath { bus_number: 1, port_numbers: vec![1, 2] });
        "2 ports"
    )]
    #[test_case(
        "/sys/bus/usb/devices/1-1.2.3.4.5.6.7",
        Some(DevicePath { bus_number: 1, port_numbers: vec![1, 2, 3, 4, 5, 6, 7] });
        "7 ports"
    )]
    #[test_case("/sys/bus/usb/devices/1-1.2.3.4.5.6.7.8", None; "8 ports")]
    #[test_case("/sys/bus/usb/devices/X-1", None; "letter")]
    #[test_case("/sys/bus/usb/devices/1-X", None; "letter2")]
    #[test_case("/sys/bus/usb/devices/1-1.X", None; "letter3")]
    #[test_case("/sys/bus/usb/devices/1-1.2.3-1", None; "3 parts")]
    fn test_parse_device_path(input: &str, want: Option<DevicePath>) {
        assert_eq!(want, DevicePath::new(input))
    }
}
