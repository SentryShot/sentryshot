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
} CDetector;

CDetector *c_detector_allocate() {
  //
  return malloc(sizeof(CDetector));
}

int c_detector_load_model(CDetector *d, const char *model_path,
                          size_t *input_tensor_size) {
#define ERROR_CREATE_FROM_FILE 10000;
#define ERROR_INTERPRETER_CREATE 10001;
#define ERROR_INPUT_TENSOR_COUNT 10002;
#define ERROR_INPUT_TENSOR_TYPE 10003;
#define ERROR_OUTPUT_TENSOR_COUNT 10004;

  int ret;

  // Load model.
  TfLiteModel *model =
      TfLiteModelCreateFromFileWithErrorReporter(model_path, reporter, NULL);
  if (model == NULL) {
    return ERROR_CREATE_FROM_FILE;
  }

  // Create interpreter from model and options.
  TfLiteInterpreterOptions *options = TfLiteInterpreterOptionsCreate();
  TfLiteInterpreterOptionsSetNumThreads(options, 1);
  TfLiteInterpreterOptionsSetErrorReporter(options, reporter, NULL);
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
  TfLiteInterpreterDelete(d->interpreter);
  free(d);
}
