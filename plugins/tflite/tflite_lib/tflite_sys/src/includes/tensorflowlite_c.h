/* Copyright 2018 The TensorFlow Authors. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License. */

#include <stdarg.h>
#include <stdint.h>
#include <stdlib.h>

/// TfLiteModel wraps a loaded TensorFlow Lite model.
typedef struct TfLiteModel TfLiteModel;

/// Same as `TfLiteModelCreateFromFile` with customizble error reporter.
/// * `reporter` takes the provided `user_data` object, as well as a C-style
///   format string and arg list (see also vprintf).
/// * `user_data` is optional. If non-null, it is owned by the client and must
///   remain valid for the duration of the interpreter lifetime.
extern TfLiteModel *TfLiteModelCreateFromFileWithErrorReporter(
    const char *model_path,
    void (*reporter)(void *user_data, const char *format, va_list args),
    void *user_data);

/// Destroys the model instance.
extern void TfLiteModelDelete(TfLiteModel *model);

/// TfLiteInterpreterOptions allows customized interpreter configuration.
typedef struct TfLiteInterpreterOptions TfLiteInterpreterOptions;

/// Returns a new interpreter options instances.
extern TfLiteInterpreterOptions *TfLiteInterpreterOptionsCreate();

/// Destroys the interpreter options instance.
extern void TfLiteInterpreterOptionsDelete(TfLiteInterpreterOptions *options);

/// Sets the number of CPU threads to use for the interpreter.
extern void
TfLiteInterpreterOptionsSetNumThreads(TfLiteInterpreterOptions *options,
                                      int32_t num_threads);

/// Sets a custom error reporter for interpreter execution.
///
/// * `reporter` takes the provided `user_data` object, as well as a C-style
///   format string and arg list (see also vprintf).
/// * `user_data` is optional. If non-null, it is owned by the client and must
///   remain valid for the duration of the interpreter lifetime.
extern void TfLiteInterpreterOptionsSetErrorReporter(
    TfLiteInterpreterOptions *options,
    void (*reporter)(void *user_data, const char *format, va_list args),
    void *user_data);

/// TfLiteInterpreter provides inference from a provided model.
typedef struct TfLiteInterpreter TfLiteInterpreter;

/// Returns a new interpreter using the provided model and options, or null on
/// failure.
///
/// * `model` must be a valid model instance. The caller retains ownership of
///   the object, and may destroy it (via TfLiteModelDelete) immediately after
///   creating the interpreter.  However, if the TfLiteModel was allocated with
///   TfLiteModelCreate, then the `model_data` buffer that was passed to
///   TfLiteModelCreate must outlive the lifetime of the TfLiteInterpreter
///   object that this function returns, and must not be modified during that
///   time; and if the TfLiteModel was allocated with TfLiteModelCreateFromFile,
///   then the contents of the model file must not be modified during the
///   lifetime of the TfLiteInterpreter object that this function returns.
/// * `optional_options` may be null. The caller retains ownership of the
///   object, and can safely destroy it (via TfLiteInterpreterOptionsDelete)
///   immediately after creating the interpreter.
///
/// \note The client *must* explicitly allocate tensors before attempting to
/// access input tensor data or invoke the interpreter.
extern TfLiteInterpreter *
TfLiteInterpreterCreate(const TfLiteModel *model,
                        const TfLiteInterpreterOptions *optional_options);

/// Destroys the interpreter.
extern void TfLiteInterpreterDelete(TfLiteInterpreter *interpreter);

/// Updates allocations for all tensors, resizing dependent tensors using the
/// specified input tensor dimensionality.
///
/// This is a relatively expensive operation, and need only be called after
/// creating the graph and/or resizing any inputs.
extern int TfLiteInterpreterAllocateTensors(TfLiteInterpreter *interpreter);

/// Returns the number of input tensors associated with the model.
extern int32_t
TfLiteInterpreterGetInputTensorCount(const TfLiteInterpreter *interpreter);

/// Returns the number of output tensors associated with the model.
extern int32_t
TfLiteInterpreterGetOutputTensorCount(const TfLiteInterpreter *interpreter);

/// Runs inference for the loaded graph.
///
/// Before calling this function, the caller should first invoke
/// TfLiteInterpreterAllocateTensors() and should also set the values for the
/// input tensors.  After successfully calling this function, the values for the
/// output tensors will be set.
///
/// \note It is possible that the interpreter is not in a ready state to
/// evaluate (e.g., if AllocateTensors() hasn't been called, or if a
/// ResizeInputTensor() has been performed without a subsequent call to
/// AllocateTensors()).
///
/// Returns one of the following status codes:
///  - kTfLiteOk: Success. Output is valid.
///  - kTfLiteDelegateError: Execution with delegates failed, due to a problem
///    with the delegate(s). If fallback was not enabled, output is invalid.
///    If fallback was enabled, this return value indicates that fallback
///    succeeded, the output is valid, and all delegates previously applied to
///    the interpreter have been undone.
///  - kTfLiteApplicationError: Same as for kTfLiteDelegateError, except that
///    the problem was not with the delegate itself, but rather was
///    due to an incompatibility between the delegate(s) and the
///    interpreter or model.
///  - kTfLiteError: Unexpected/runtime failure. Output is invalid.
extern int TfLiteInterpreterInvoke(TfLiteInterpreter *interpreter);

/// A tensor in the interpreter system which is a wrapper around a buffer of
/// data including a dimensionality (or NULL if not currently defined).
typedef struct TfLiteTensor TfLiteTensor;

/// Free memory of tensor `t`.
void TfLiteTensorFree(TfLiteTensor *t);

/// Returns the tensor associated with the input index.
/// REQUIRES: 0 <= input_index < TfLiteInterpreterGetInputTensorCount(tensor)
extern TfLiteTensor *
TfLiteInterpreterGetInputTensor(const TfLiteInterpreter *interpreter,
                                int32_t input_index);

/// Returns the tensor associated with the output index.
/// REQUIRES: 0 <= output_index < TfLiteInterpreterGetOutputTensorCount(tensor)
///
/// \note The shape and underlying data buffer for output tensors may be not
/// be available until after the output tensor has been both sized and
/// allocated.
/// In general, best practice is to interact with the output tensor *after*
/// calling TfLiteInterpreterInvoke().
extern const TfLiteTensor *
TfLiteInterpreterGetOutputTensor(const TfLiteInterpreter *interpreter,
                                 int32_t output_index);

/// Returns the type of a tensor element.
extern int TfLiteTensorType(const TfLiteTensor *tensor);

/// Returns the number of dimensions that the tensor has.  Returns -1 in case
/// the 'opaque_tensor' does not have its dimensions property set.
extern int32_t TfLiteTensorNumDims(const TfLiteTensor *tensor);

/// Returns the length of the tensor in the "dim_index" dimension.
/// REQUIRES: 0 <= dim_index < TFLiteTensorNumDims(tensor)
extern int32_t TfLiteTensorDim(const TfLiteTensor *tensor, int32_t dim_index);

/// Returns the size of the underlying data in bytes.
extern size_t TfLiteTensorByteSize(const TfLiteTensor *tensor);

/// Copies from the provided input buffer into the tensor's buffer.
/// REQUIRES: input_data_size == TfLiteTensorByteSize(tensor)
extern int TfLiteTensorCopyFromBuffer(TfLiteTensor *tensor,
                                      const void *input_data,
                                      size_t input_data_size);

/// Returns a pointer to the underlying data buffer.
///
/// \note The result may be null if tensors have not yet been allocated, e.g.,
/// if the Tensor has just been created or resized and `TfLiteAllocateTensors()`
/// has yet to be called, or if the output tensor is dynamically sized and the
/// interpreter hasn't been invoked.
extern void *TfLiteTensorData(const TfLiteTensor *tensor);

// EDGETPU.

/// TfLiteOpaqueDelegateStruct: unconditionally opaque version of
/// TfLiteDelegate; allows delegation of nodes to alternative backends.
///
/// This is an abstract type that is intended to have the same
/// role as TfLiteDelegate, but without exposing the implementation
/// details of how delegates are implemented.
///
/// WARNING: This is an experimental type and subject to change.
typedef struct TfLiteOpaqueDelegateStruct TfLiteOpaqueDelegateStruct;

/// TfLiteOpaqueDelegate: conditionally opaque version of
/// TfLiteDelegate; allows delegation of nodes to alternative backends.
/// For TF Lite in Play Services, this is an opaque type,
/// but for regular TF Lite, this is just a typedef for TfLiteDelegate.
///
/// WARNING: This is an experimental type and subject to change.
typedef TfLiteOpaqueDelegateStruct TfLiteOpaqueDelegate;

/// Adds a delegate to be applied during `TfLiteInterpreter` creation.
///
/// If delegate application fails, interpreter creation will also fail with an
/// associated error logged.
///
/// \note The caller retains ownership of the delegate and should ensure that it
/// remains valid for the duration of any created interpreter's lifetime.
///
/// If you are NOT using "TensorFlow Lite in Play Services", and NOT building
/// with `TFLITE_WITH_STABLE_ABI` or `TFLITE_USE_OPAQUE_DELEGATE` macros
/// enabled, it is possible to pass a `TfLiteDelegate*` rather than a
/// `TfLiteOpaqueDelegate*` to this function, since in those cases,
/// `TfLiteOpaqueDelegate` is just a typedef alias for `TfLiteDelegate`.
/// This is for compatibility with existing source code
/// and existing delegates.  For new delegates, it is recommended to
/// use `TfLiteOpaqueDelegate` rather than `TfLiteDelegate`.  (See
/// `TfLiteOpaqueDelegate` in tensorflow/lite/core/c/c_api_types.h.)
extern void
TfLiteInterpreterOptionsAddDelegate(TfLiteInterpreterOptions *options,
                                    TfLiteOpaqueDelegate *delegate);

enum edgetpu_device_type {
  EDGETPU_APEX_PCI = 0,
  EDGETPU_APEX_USB = 1,
};

struct edgetpu_option {
  const char *name;
  const char *value;
};

// Creates a delegate which handles all edge TPU custom ops inside
// `tflite::Interpreter`. Options must be available only during the call of this
// function.
TfLiteOpaqueDelegate *
edgetpu_create_delegate(enum edgetpu_device_type type, const char *name,
                        const struct edgetpu_option *options,
                        size_t num_options);

// Frees delegate returned by `edgetpu_create_delegate`.
void edgetpu_free_delegate(TfLiteOpaqueDelegate *delegate);

struct edgetpu_device {
  enum edgetpu_device_type type;
  const char *path;
};

// Returns array of connected edge TPU devices.
struct edgetpu_device *edgetpu_list_devices(size_t *num_devices);

// Frees array returned by `edgetpu_list_devices`.
void edgetpu_free_devices(struct edgetpu_device *dev);

// Sets verbosity of operating logs related to edge TPU.
// Verbosity level can be set to [0-10], in which 10 is the most verbose.
void edgetpu_verbosity(int verbosity);
