# 8. Handling the Tokio Runtime in Plugins

This document outlines the design for handling asynchronous operations within a SentryShot plugin, specifically addressing the challenges of the Tokio runtime's context when interacting with the synchronous plugin loading system.

## 1. The Problem: Async Initialization in a Sync Context

The SentryShot plugin system loads plugins at startup using a synchronous `load` function. This function is defined in each plugin's shared library and must return a fully created plugin instance wrapped in an `Arc`.

However, many plugins, especially those involving network communication like the `openvino` plugin, need to perform `async` operations for their initialization (e.g., establishing a gRPC connection to a model server).

This leads to a conflict:

1.  **The `load` function is synchronous**, but the initialization logic is `async`.
2.  Attempting to use `rt_handle.block_on()` inside the `load` function is a known anti-pattern. If the host application calls `load` from within its own async context, it will cause the application to panic.
3.  Empirical testing has shown that the `async` plugin hooks, such as `on_monitor_start`, are **not** guaranteed to execute within a valid Tokio runtime context. Calls to `tokio::runtime::Handle::try_current()` from within this hook can fail with a "no reactor running" error. This means we cannot rely on the host to poll the plugin's futures correctly.

## 2. The Solution: A Self-Contained Plugin

To solve these issues, the plugin must become self-contained in its asynchronous execution. The chosen design is for the plugin to **create, own, and manage its own dedicated Tokio runtime**.

This approach has several key benefits:

*   **Robustness:** The plugin's async operations are completely independent of how the host application manages its threads or runtimes. This makes the plugin resilient to changes or issues in the host's architecture.
*   **Correctness:** It completely avoids the `block_on` anti-pattern by providing a valid runtime to block on during the synchronous `load` phase.
*   **Encapsulation:** The plugin cleanly manages its own async responsibilities. All async tasks required by the plugin will be scheduled on its own internal, guaranteed-valid runtime.

## 3. Implementation Plan for the `openvino` Plugin

Here is the step-by-step plan to refactor the `openvino` plugin to use this self-contained design.

### Step 1: The Plugin Owns the Runtime

The `OpenvinoPlugin` struct will be modified to hold an instance of `tokio::runtime::Runtime`. This solves the lifetime problem of creating a runtime inside `load`, as the runtime will now live as long as the plugin itself.

```rust
// In plugins/openvino/openvino.rs

struct OpenvinoPlugin {
    runtime: tokio::runtime::Runtime,
    logger: ArcLogger,
    detector_manager: DetectorManager,
}
```

### Step 2: Synchronous Load with Async Initialization

The `load` function will be responsible for setting up the runtime and using it to initialize all async components before creating the plugin instance.

```rust
// In plugins/openvino/openvino.rs

#[unsafe(no_mangle)]
pub extern "Rust" fn load(app: &dyn Application) -> Arc<dyn Plugin> {
    // 1. Create a new, dedicated Tokio runtime for this plugin.
    let runtime = tokio::runtime::Runtime::new().unwrap();

    // 2. Use the new runtime's `block_on` to initialize the DetectorManager.
    let detector_manager = runtime.block_on(async {
        let config = match app.env().raw().parse::<Config>() {
            Ok(config) => config,
            Err(e) => {
                eprintln!("failed to parse openvino config: {e}");
                std::process::exit(1);
            }
        };
        let openvino_logger = Arc::new(OpenvinoLogger {
            logger: app.logger(),
        });
        DetectorManager::new(openvino_logger, &config).await
    });

    // 3. Create the plugin instance synchronously, moving the runtime into it.
    let plugin = OpenvinoPlugin {
        runtime,
        logger: app.logger(),
        detector_manager,
    };

    Arc::new(plugin)
}
```

### Step 3: Execute Async Hooks on the Plugin's Own Runtime

Since we can't trust the host to poll the plugin's `async` methods, we must use our own runtime. Inside `on_monitor_start`, we will use `self.runtime.spawn()` to run the core plugin logic on a valid executor thread.

```rust
// In plugins/openvino/openvino.rs

#[async_trait]
impl Plugin for OpenvinoPlugin {
    async fn on_monitor_start(&self, token: CancellationToken, monitor: ArcMonitor) {
        // Because the caller doesn't provide a runtime, we spawn the actual
        // logic onto our own, self-contained runtime.
        let detector_manager = self.detector_manager.clone(); // Arc
        let logger = self.logger.clone();

        self.runtime.spawn(async move {
            let msg_logger = Arc::new(OpenvinoMsgLogger {
                logger,
                monitor_id: monitor.config().id().to_owned(),
            });

            if let Err(e) = start(token, msg_logger.clone(), monitor, detector_manager).await {
                msg_logger.log(LogLevel::Error, &format!("start: {e}"));
            };
        });
    }
}
```

This design makes the `openvino` plugin robust and correctly handles the complexities of the SentryShot plugin system's threading model.