// Run "./bindgen.sh" after modifying this file!

#include <stddef.h>
#include <stdint.h>

typedef struct CDetector CDetector;

CDetector *c_detector_allocate();

typedef struct TfLiteTensor TfLiteTensor;

enum edgetpu_device_type {
  EDGETPU_APEX_PCI = 0,
  EDGETPU_APEX_USB = 1,
};

extern int TfLiteTensorType(const TfLiteTensor *tensor);
extern int32_t TfLiteTensorNumDims(const TfLiteTensor *tensor);
extern int32_t TfLiteTensorDim(const TfLiteTensor *tensor, int32_t dim_index);
extern size_t TfLiteTensorByteSize(const TfLiteTensor *tensor);

int c_detector_load_model(CDetector *d, const char *model_path,
                          const char *device,
                          const enum edgetpu_device_type device_type);

int32_t c_detector_input_tensor_count(CDetector *d);
int32_t c_detector_output_tensor_count(CDetector *d);

TfLiteTensor *c_detector_input_tensor(CDetector *d, int32_t index);
const TfLiteTensor *c_detector_output_tensor(CDetector *d, int32_t index);

extern int TfLiteTensorCopyFromBuffer(TfLiteTensor *tensor,
                                      const void *input_data,
                                      size_t input_data_size);

int c_detector_invoke_interpreter(CDetector *d);

extern void *TfLiteTensorData(const TfLiteTensor *tensor);

void c_detector_free(CDetector *d);

struct edgetpu_device {
  enum edgetpu_device_type type;
  const char *path;
};

// Returns array of connected edge TPU devices.
struct edgetpu_device *c_list_devices(size_t *num_devices);

// Frees array returned by `list_devices`.
void c_free_devices(struct edgetpu_device *dev);

int c_probe_device(int *ret, const int device_bus_number,
                   const int device_ports_len, const uint8_t *device_ports);

void c_poke_devices();
