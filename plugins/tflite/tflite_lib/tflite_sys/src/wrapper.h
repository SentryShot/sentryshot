// Run "./bindgen.sh" after modifying this file!

#include <stddef.h>
#include <stdint.h>

typedef struct CDetector CDetector;

CDetector *c_detector_allocate();

int c_detector_load_model(CDetector *d, const char *model_path,
                          size_t *input_tensor_size);

int c_detector_detect(CDetector *d, const uint8_t *buf, size_t buf_size,
                      uint8_t **t0_data, uint8_t **t1_data, uint8_t **t2_data,
                      uint8_t **t3_data, size_t *t0_size, size_t *t1_size,
                      size_t *t2_size, size_t *t3_size);

void c_detector_free(CDetector *d);
