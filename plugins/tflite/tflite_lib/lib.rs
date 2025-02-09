use std::{
    ffi::{c_uint, CStr, CString, NulError},
    fmt::{Debug, Display, Formatter},
    num::NonZeroU16,
    os::raw::c_int,
    path::{Path, PathBuf},
    process::{Command, Stdio},
    slice::{self, from_raw_parts},
    str::FromStr,
    time::Duration,
};
use tflite_sys::{
    c_detector_allocate, c_detector_free, c_detector_input_tensor, c_detector_input_tensor_count,
    c_detector_invoke_interpreter, c_detector_load_model, c_detector_output_tensor,
    c_detector_output_tensor_count, c_free_devices, c_list_devices, c_poke_devices, c_probe_device,
    CDetector, TfLiteTensor, TfLiteTensorByteSize, TfLiteTensorCopyFromBuffer, TfLiteTensorData,
    TfLiteTensorDim, TfLiteTensorNumDims, TfLiteTensorType,
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

    #[error("create edgetpu delegate")]
    EdgetpuDelegateCreate,

    #[error("load model: {0}")]
    LoadModel(i32),

    #[error("probe device: {0}, '{1}'")]
    ProbeDevice(ProbeDeviceError, String),

    #[error("debug device: {0}")]
    DebugDevice(#[from] DebugDeviceError),

    #[error("invalid input tensor count: {0}")]
    InvalidInputTensorCount(i32),

    #[error("invalid output tensor count: {0}")]
    InvalidOutputTensorCount(i32),

    #[error("mismatched input tensor shapes: config={0:?}, model={1:?}")]
    MismatchedInputShapes(String, String),

    #[error("mismatched output tensor shape: config={0:?}, model={1:?}")]
    MismatchedOutputShape(String, String),
}

const ERROR_CREATE_FROM_FILE: c_int = 10000;
const ERROR_INTERPRETER_CREATE: c_int = 10001;
const ERROR_EDGETPU_DELEGATE_CREATE: c_int = 10002;

#[derive(Debug, Error)]
#[error("invoke interpreter: {0}")]
pub struct InvokeInterpreterError(i32);

// Safe wrapper for C detector with loaded model.
struct RDetector {
    inner: *mut CDetector,
    input_tensors: Vec<InputTensor>,
    output_tensors: Vec<OutputTensor>,
}

impl Drop for RDetector {
    fn drop(&mut self) {
        unsafe { c_detector_free(self.inner) }
    }
}

unsafe impl Send for RDetector {}

impl RDetector {
    fn new(
        model_path: &Path,
        edgetpu: Option<&EdgetpuDevice>,
        width: NonZeroU16,
        height: NonZeroU16,
    ) -> Result<Self, NewDetectorError> {
        use NewDetectorError::*;
        let model_path = model_path
            .to_str()
            .ok_or_else(|| NewDetectorError::PathToString(model_path.to_path_buf()))?;
        let model_path = CString::new(model_path)?;

        let c_detector = unsafe { c_detector_allocate() };
        if c_detector.is_null() {
            return Err(DetectorNull);
        }

        let res = match edgetpu {
            Some(device) => {
                if let Err(e) = probe_device(&device.path) {
                    return Err(ProbeDevice(e, device.path.clone()));
                };
                let path = CString::new(device.path.clone())?;
                unsafe {
                    c_detector_load_model(
                        c_detector,
                        model_path.as_ptr(),
                        path.as_ptr(),
                        device.typ.as_uint(),
                    )
                }
            }
            None => unsafe {
                c_detector_load_model(c_detector, model_path.as_ptr(), std::ptr::null(), 0)
            },
        };
        if res != 0 {
            return Err(match res {
                ERROR_CREATE_FROM_FILE => CreateFromFile,
                ERROR_INTERPRETER_CREATE => InterpreterCreate,
                ERROR_EDGETPU_DELEGATE_CREATE => EdgetpuDelegateCreate,
                _ => LoadModel(res),
            });
        }

        let input_tensor_count = unsafe { c_detector_input_tensor_count(c_detector) };
        let input_tensor_count = u8::try_from(input_tensor_count)
            .map_err(|_| InvalidInputTensorCount(input_tensor_count))?;

        let input_tensors: Vec<InputTensor> = (0..input_tensor_count)
            .map(|i| unsafe { InputTensor::new(c_detector, i) })
            .collect();

        let output_tensor_count = unsafe { c_detector_output_tensor_count(c_detector) };
        let output_tensor_count = u8::try_from(output_tensor_count)
            .map_err(|_| InvalidInputTensorCount(output_tensor_count))?;

        let output_tensors: Vec<OutputTensor> = (0..output_tensor_count)
            .map(|i| unsafe { OutputTensor::new(c_detector, i) })
            .collect();

        let expected_input_shapes = format!("(uint8[1, {height}, {width}, 3])");
        let expected_output_shape = "(float32, float32, float32, float32)".to_owned();

        let input_shapes = format_list(input_tensors.iter().map(InputTensor::shape));
        let output_shape = format_list(output_tensors.iter().map(|t| t.typ));

        if expected_input_shapes != input_shapes {
            return Err(MismatchedInputShapes(expected_input_shapes, input_shapes));
        }
        if expected_output_shape != output_shape {
            return Err(MismatchedOutputShape(expected_output_shape, output_shape));
        }

        Ok(Self {
            inner: c_detector,
            input_tensors,
            output_tensors,
        })
    }

    fn invoke_interpreter(&mut self) -> Result<(), InvokeInterpreterError> {
        let res = unsafe { c_detector_invoke_interpreter(self.inner) };
        if res != 0 {
            return Err(InvokeInterpreterError(res));
        }
        Ok(())
    }
}

#[derive(Debug, Error)]
pub enum WriteTensorError {
    #[error("buffer size: {0}vs{1}")]
    BufferSize(usize, usize),

    #[error("write error: {0}")]
    Write(i32),
}

struct InputTensor {
    inner: *mut TfLiteTensor,
    size: usize,
    typ: TensorType,
    dims: Vec<i32>,
}

impl InputTensor {
    unsafe fn new(c_detector: *mut CDetector, index: u8) -> Self {
        let tensor = unsafe { c_detector_input_tensor(c_detector, index.into()) };
        assert!(!tensor.is_null());
        let size = unsafe { TfLiteTensorByteSize(tensor) };
        let typ = unsafe { TensorType::from_tensor(tensor) };
        let num_dims = unsafe { TfLiteTensorNumDims(tensor) };
        let dims = (0..num_dims).map(|i| TfLiteTensorDim(tensor, i)).collect();
        Self {
            inner: tensor,
            size,
            typ,
            dims,
        }
    }

    fn shape(&self) -> String {
        format!("{}{:?}", self.typ, self.dims)
    }

    fn write_u8(&mut self, buf: &[u8]) -> Result<(), WriteTensorError> {
        use WriteTensorError::*;
        assert!(matches!(self.typ, TensorType::UInt8));
        if self.size != buf.len() {
            return Err(BufferSize(self.size, buf.len()));
        }
        let ret = unsafe {
            TfLiteTensorCopyFromBuffer(self.inner.cast(), buf.as_ptr().cast(), buf.len())
        };
        if ret != 0 {
            return Err(Write(ret));
        }
        Ok(())
    }
}

struct OutputTensor {
    inner: *const TfLiteTensor,
    size: usize,
    typ: TensorType,
}

impl OutputTensor {
    unsafe fn new(c_detector: *mut CDetector, index: u8) -> Self {
        let tensor = unsafe { c_detector_output_tensor(c_detector, index.into()) };
        assert!(!tensor.is_null());
        let size = unsafe { TfLiteTensorByteSize(tensor) };
        let typ = unsafe { TensorType::from_tensor(tensor) };
        Self {
            inner: tensor,
            size,
            typ,
        }
    }

    fn data_f32(&self) -> &[f32] {
        assert!(matches!(self.typ, TensorType::Float32));
        unsafe { from_raw_parts(TfLiteTensorData(self.inner).cast(), self.size / 4) }
    }
}

#[derive(Clone, Copy, PartialEq, Eq)]
pub enum TensorType {
    NoType,
    Float32,
    Int32,
    UInt8,
    Int64,
    String,
    Bool,
    Int16,
    Complex64,
    Int8,
    Unknown(i32),
}

impl Display for TensorType {
    fn fmt(&self, f: &mut Formatter<'_>) -> std::fmt::Result {
        let s = match self {
            TensorType::NoType => "none",
            TensorType::Float32 => "float32",
            TensorType::Int32 => "int32",
            TensorType::UInt8 => "uint8",
            TensorType::Int64 => "int64",
            TensorType::String => "string",
            TensorType::Bool => "boolean",
            TensorType::Int16 => "int16",
            TensorType::Complex64 => "complex64",
            TensorType::Int8 => "int8",
            TensorType::Unknown(v) => return f.write_str(&v.to_string()),
        };
        f.write_str(s)
    }
}

impl TensorType {
    unsafe fn from_tensor(tensor: *const TfLiteTensor) -> Self {
        let v = unsafe { TfLiteTensorType(tensor) };
        match v {
            0 => TensorType::NoType,
            1 => TensorType::Float32,
            2 => TensorType::Int32,
            3 => TensorType::UInt8,
            4 => TensorType::Int64,
            5 => TensorType::String,
            6 => TensorType::Bool,
            7 => TensorType::Int16,
            8 => TensorType::Complex64,
            9 => TensorType::Int8,
            _ => TensorType::Unknown(v),
        }
    }
}

#[derive(Debug, Error)]
pub enum DetectError {
    #[error(transparent)]
    WriteTensor(#[from] WriteTensorError),

    #[error(transparent)]
    Invoke(#[from] InvokeInterpreterError),

    #[error("output tensor type")]
    OutputTensorType,

    #[error("detect: {0}")]
    Detect(i32),

    #[error("parse output tensors: {0:?} {1}")]
    ParseOutputTensors(String, ParseOutputTensorsError),
}

pub struct Detector {
    detector: RDetector,
}

impl Detector {
    pub fn new(
        model_path: &Path,
        edgetpu: Option<&EdgetpuDevice>,
        width: NonZeroU16,
        height: NonZeroU16,
    ) -> Result<Self, NewDetectorError> {
        let detector = RDetector::new(model_path, edgetpu, width, height)?;
        Ok(Self { detector })
    }

    pub fn detect(&mut self, buf: &[u8]) -> Result<Vec<Detection>, DetectError> {
        use DetectError::*;

        self.detector.input_tensors[0].write_u8(buf)?;
        self.detector.invoke_interpreter()?;

        let t0 = self.detector.output_tensors[0].data_f32();
        let t1 = self.detector.output_tensors[1].data_f32();
        let t2 = self.detector.output_tensors[2].data_f32();
        let t3 = self.detector.output_tensors[3].data_f32();
        parse_output_tensors(t0, t1, t2, t3).map_err(|e| {
            ParseOutputTensors(
                format!("[{}, {}, {}, {}]", t0.len(), t1.len(), t2.len(), t3.len()),
                e,
            )
        })
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

#[allow(
    clippy::cast_sign_loss,
    clippy::cast_possible_truncation,
    clippy::as_conversions
)]
fn parse_output_tensors(
    t0: &[f32],
    t1: &[f32],
    t2: &[f32],
    t3: &[f32],
) -> Result<Vec<Detection>, ParseOutputTensorsError> {
    use ParseOutputTensorsError::*;

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

        let score = score.max(0.0).min(1.0);
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

fn format_list<T: Display, I: Iterator<Item = T>>(v: I) -> String {
    let values: Vec<String> = v.map(|v| v.to_string()).collect();
    format!("({})", values.join(", "))
}

// All values besides class are between zero and one.
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
    fn as_uint(self) -> c_uint {
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

#[must_use]
pub fn debug_device(path: String, devices: &[EdgetpuDevice]) -> DebugDeviceError {
    use DebugDeviceError::*;
    print_device(&path, devices);
    if !Path::new(&path).exists() {
        return DeviceNotFound(path);
    }
    Exists(path)
}

fn print_device(path: &str, devices: &[EdgetpuDevice]) {
    println!("Found {} edgetpu devices", devices.len());
    for device in devices {
        println!("{device}");
    }
    let Some(parent) = Path::new(path).parent() else {
        println!("device path does not have a parent: {path:?}");
        return;
    };
    let parent = parent.to_str().unwrap_or("");
    println!("ls -la {parent}");
    let result = Command::new("ls")
        .arg("-la")
        .arg(parent)
        .stdout(Stdio::inherit())
        .stderr(Stdio::inherit())
        .spawn();
    if let Err(e) = result {
        println!("{e}");
    }
    std::thread::sleep(Duration::from_millis(100));
}

#[must_use]
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
            let path = path
                .to_str()
                .expect("libedgetpu returned a device path that isn't a valid string: {path:?}")
                .to_owned();
            devices.push(EdgetpuDevice { typ, path });
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
    let Some(device_path) = DevicePath::new(path) else {
        return Err(ParsePath);
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
            _ => panic!("unexpected error code: {err}"),
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
            .map(str::parse)
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

#[allow(clippy::needless_pass_by_value)]
#[cfg(test)]
mod tests {
    use super::*;
    use test_case::test_case;

    #[allow(clippy::needless_pass_by_value)]
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
        assert_eq!(want, DevicePath::new(input));
    }
}
