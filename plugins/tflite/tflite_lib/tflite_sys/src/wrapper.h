// Run "./bindgen.sh" after modifying this file!

#include <stddef.h>
#include <stdint.h>

typedef struct CDetector CDetector;

CDetector *c_detector_allocate();

enum edgetpu_device_type {
  EDGETPU_APEX_PCI = 0,
  EDGETPU_APEX_USB = 1,
};

int c_detector_load_model(CDetector *d, const char *model_path,
                          size_t *input_tensor_size, const char *device,
                          const enum edgetpu_device_type device_type);

int c_detector_detect(CDetector *d, const uint8_t *buf, size_t buf_size,
                      uint8_t **t0_data, uint8_t **t1_data, uint8_t **t2_data,
                      uint8_t **t3_data, size_t *t0_size, size_t *t1_size,
                      size_t *t2_size, size_t *t3_size);

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
