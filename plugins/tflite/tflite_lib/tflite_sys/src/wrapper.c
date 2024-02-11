#include <libusb-1.0/libusb.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <tensorflowlite_c.h>

void reporter(void *user_data, const char *format, va_list args) {
  (void)user_data;
  const char *prefix = "TFLITE ERROR: ";
  char *f;
  f = malloc(strlen(prefix) + strlen(format) + 1);
  strcpy(f, prefix);
  strcat(f, format);
  fprintf(stderr, f, args);
  free(f);
}

typedef struct {
  TfLiteInterpreter *interpreter;
  TfLiteTensor *input_tensor;
  TfLiteOpaqueDelegate *delegate;
} CDetector;

CDetector *c_detector_allocate() {
  CDetector *d = malloc(sizeof(CDetector));
  d->interpreter = NULL;
  d->input_tensor = NULL;
  d->delegate = NULL;
  return d;
}

int c_detector_load_model(CDetector *d, const char *model_path,
                          size_t *input_tensor_size, const char *device,
                          const enum edgetpu_device_type device_type) {
#define ERROR_CREATE_FROM_FILE 10000;
#define ERROR_INTERPRETER_CREATE 10001;
#define ERROR_INPUT_TENSOR_COUNT 10002;
#define ERROR_INPUT_TENSOR_TYPE 10003;
#define ERROR_OUTPUT_TENSOR_COUNT 10004;
#define ERROR_EDGETPU_DELEGATE_CREATE 10005;

  int ret;

  // Load model.
  TfLiteModel *model =
      TfLiteModelCreateFromFileWithErrorReporter(model_path, reporter, NULL);
  if (model == NULL) {
    return ERROR_CREATE_FROM_FILE;
  }

  // Create interpreter.
  TfLiteInterpreterOptions *options = TfLiteInterpreterOptionsCreate();
  TfLiteInterpreterOptionsSetNumThreads(options, 1);
  TfLiteInterpreterOptionsSetErrorReporter(options, reporter, NULL);
  if (device != NULL) {
    // Create edgetpu delegate.
    d->delegate = edgetpu_create_delegate(device_type, device, NULL, 0);
    if (d->delegate == NULL) {
      return ERROR_EDGETPU_DELEGATE_CREATE
    }
    TfLiteInterpreterOptionsAddDelegate(options, d->delegate);
  }

  d->interpreter = TfLiteInterpreterCreate(model, options);
  if (d->interpreter == NULL) {
    return ERROR_INTERPRETER_CREATE
  }
  TfLiteModelDelete(model);
  TfLiteInterpreterOptionsDelete(options);

  // Allocate tensors.
  if ((ret = TfLiteInterpreterAllocateTensors(d->interpreter)) != 0) {
    return ret;
  }

  int32_t inputTensorCount =
      TfLiteInterpreterGetInputTensorCount(d->interpreter);
  if (inputTensorCount != 1) {
    return ERROR_INPUT_TENSOR_COUNT;
  }

  d->input_tensor = TfLiteInterpreterGetInputTensor(d->interpreter, 0);

  *input_tensor_size = TfLiteTensorByteSize(d->input_tensor);

  int input_tensor_type = TfLiteTensorType(d->input_tensor);
  if (input_tensor_type != 3) {
    return ERROR_INPUT_TENSOR_TYPE;
  }

  int32_t output_tensor_count =
      TfLiteInterpreterGetOutputTensorCount(d->interpreter);
  if (output_tensor_count != 4) {
    return ERROR_OUTPUT_TENSOR_COUNT;
  }

  return 0;
}

int c_detector_detect(CDetector *d, const uint8_t *buf, size_t buf_size,
                      uint8_t **t0_data, uint8_t **t1_data, uint8_t **t2_data,
                      uint8_t **t3_data, size_t *t0_size, size_t *t1_size,
                      size_t *t2_size, size_t *t3_size) {
#define ERROR_OUTPUT_TENSOR_TYPE 20000;
  // Populate input tensor data.
  int ret;
  if ((ret = TfLiteTensorCopyFromBuffer(d->input_tensor, buf, buf_size)) != 0) {
    return ret;
  }

  // Execute inference.
  if ((ret = TfLiteInterpreterInvoke(d->interpreter)) != 0) {
    return ret;
  }

  const TfLiteTensor *t0 = TfLiteInterpreterGetOutputTensor(d->interpreter, 0);
  const TfLiteTensor *t1 = TfLiteInterpreterGetOutputTensor(d->interpreter, 1);
  const TfLiteTensor *t2 = TfLiteInterpreterGetOutputTensor(d->interpreter, 2);
  const TfLiteTensor *t3 = TfLiteInterpreterGetOutputTensor(d->interpreter, 3);

  if (TfLiteTensorType(t0) != 1 && TfLiteTensorType(t1) != 1 &&
      TfLiteTensorType(t2) != 1 && TfLiteTensorType(t3) != 1) {
    return ERROR_OUTPUT_TENSOR_TYPE;
  }

  *t0_data = TfLiteTensorData(t0);
  *t1_data = TfLiteTensorData(t1);
  *t2_data = TfLiteTensorData(t2);
  *t3_data = TfLiteTensorData(t3);

  *t0_size = TfLiteTensorByteSize(t0);
  *t1_size = TfLiteTensorByteSize(t1);
  *t2_size = TfLiteTensorByteSize(t2);
  *t3_size = TfLiteTensorByteSize(t3);

  return 0;
}

void c_detector_free(CDetector *d) {
  if (d->interpreter != NULL) {
    TfLiteInterpreterDelete(d->interpreter);
  }
  if (d->input_tensor != NULL) {
    TfLiteTensorFree(d->input_tensor);
  }
  if (d->delegate != NULL) {
    edgetpu_free_delegate(d->delegate);
  }
  free(d);
}

// Returns array of connected edge TPU devices.
struct edgetpu_device *c_list_devices(size_t *num_devices) {
  return edgetpu_list_devices(num_devices);
};

// Frees array returned by `list_devices`.
void c_free_devices(struct edgetpu_device *dev) {
  //
  edgetpu_free_devices(dev);
}

int c_probe_device(int *ret, const int device_bus_number,
                   const int device_ports_len, const uint8_t *device_ports) {

#define ERROR_USB_INIT 20000;
#define ERROR_USB_GET_DEVICE_LIST 20001;
#define ERROR_USB_GET_PORT_NUMBERS 20002;
#define ERROR_USB_OPEN_DEVICE 20003;
#define ERROR_USB_NOT_FOUND 20004;

#define kMaxUsbPathDepth 7

  libusb_context *context;
  *ret = libusb_init(&context);
  if (*ret < 0) {
    return ERROR_USB_INIT;
  }

  libusb_device **device_list;
  *ret = libusb_get_device_list(context, &device_list);
  if (*ret < 0) {
    libusb_exit(context);
    return ERROR_USB_GET_DEVICE_LIST;
  }
  int num_devices = *ret;

  for (int device_index = 0; device_index < num_devices; ++device_index) {
    libusb_device *device = device_list[device_index];

    const uint8_t bus_number = libusb_get_bus_number(device);
    if (bus_number != device_bus_number) {
      continue;
    }

    // Generate path string for this device.
    uint8_t port_numbers[kMaxUsbPathDepth] = {0};
    *ret = libusb_get_port_numbers(device, port_numbers, kMaxUsbPathDepth);
    if (*ret < 0) {
      libusb_free_device_list(device_list, 1);
      libusb_exit(context);
      return ERROR_USB_GET_PORT_NUMBERS;
    }
    if (*ret != device_ports_len) {
      continue;
    }

    // Compare ports.
    if (memcmp(port_numbers, device_ports, device_ports_len) == 0) {
      // Found the device, try to open it.
      libusb_device_handle *device_handle;
      *ret = libusb_open(device, &device_handle);
      if (*ret != 0) {
        libusb_free_device_list(device_list, 1);
        return ERROR_USB_OPEN_DEVICE;
      }
      // Successfully opened the device, clean up and return.
      libusb_close(device_handle);
      libusb_free_device_list(device_list, 1);
      libusb_exit(context);
      return 0;
    }
  }

  libusb_free_device_list(device_list, 1);
  libusb_exit(context);
  return ERROR_USB_NOT_FOUND;
}

void c_poke_devices() {
  size_t num_devices;
  struct edgetpu_device *devices = edgetpu_list_devices(&num_devices);

  for (size_t i = 0; i < num_devices; i++) {
    struct edgetpu_device device = devices[i];
    printf("poking device: %s\n", device.path);

    TfLiteOpaqueDelegate *delegate =
        edgetpu_create_delegate(device.type, device.path, NULL, 0);
    if (delegate != NULL) {
      continue;
    }
    edgetpu_free_delegate(delegate);
  }
}
