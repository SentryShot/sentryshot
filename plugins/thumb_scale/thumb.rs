// SPDX-License-Identifier: GPL-2.0-or-later

use async_trait::async_trait;
use common::{monitor::MonitorConfig, DynLogger, LogEntry, LogLevel};
use plugin::{types::Assets, Application, Plugin};
use sentryshot_scale::{Frame, Scaler};
use serde::Deserialize;
use std::{borrow::Cow, num::NonZeroU16, sync::Arc};

#[no_mangle]
pub extern "Rust" fn version() -> String {
    plugin::get_version()
}

#[no_mangle]
pub extern "Rust" fn load(app: &dyn Application) -> Arc<dyn Plugin> {
    Arc::new(ThumbScalePlugin {
        logger: app.logger(),
    })
}
struct ThumbScalePlugin {
    logger: DynLogger,
}
#[async_trait]
impl Plugin for ThumbScalePlugin {
    #[allow(clippy::unwrap_used)]
    fn edit_assets(&self, assets: &mut Assets) {
        let js = assets["scripts/settings.js"].to_vec();
        *assets.get_mut("scripts/settings.js").unwrap() = Cow::Owned(modify_settings_js(js));
    }

    #[allow(clippy::unwrap_used)]
    fn on_thumb_save(&self, config: &MonitorConfig, frame: Frame) -> Frame {
        let log = |level: LogLevel, msg: &str| {
            self.logger.log(LogEntry {
                level,
                source: "recorder".to_owned().try_into().unwrap(),
                monitor_id: Some(config.id().to_owned()),
                message: format!("thumb scale: {msg}").try_into().unwrap(),
            });
        };

        #[derive(Deserialize)]
        #[allow(clippy::items_after_statements)]
        struct Temp {
            #[serde(rename = "thumbScale")]
            thumb_scale: String,
        }
        let Ok(temp) = serde_json::from_value::<Temp>(config.raw().clone()) else {
            log(LogLevel::Warning, "config is not set");
            return frame;
        };
        let scale = temp.thumb_scale;
        println!("SCALE {scale}");
        let scale = match scale.as_str() {
            "full" => return frame,
            "half" => 2,
            "third" => 3,
            "quarter" => 4,
            "sixth" => 6,
            "eighth" => 8,
            _ => {
                log(LogLevel::Warning, &format!("invalid config: '{scale}'"));
                return frame;
            }
        };

        let src_width = frame.width();
        let src_height = frame.height();
        let pix_fmt = frame.pix_fmt();
        let dst_width = NonZeroU16::new(src_width.get() / scale).unwrap();
        let dst_height = src_height.get() / scale;

        // The converter doesn't suport odd heights.
        let dst_height = NonZeroU16::new(dst_height + (dst_height & 1)).unwrap();

        log(
            LogLevel::Info,
            &format!("downscaling {src_width}x{src_height} to {dst_width}x{dst_height}"),
        );

        let result = Scaler::new(src_width, src_height, pix_fmt, dst_width, dst_height);
        let mut scaler = match result {
            Ok(v) => v,
            Err(e) => {
                log(LogLevel::Error, &format!("failed to crate scaler: '{e}'"));
                return frame;
            }
        };

        let mut frame2 = Frame::new();
        if let Err(e) = scaler.scale(&frame, &mut frame2) {
            log(LogLevel::Error, &format!("failed to scale frame: '{e}'"));
            return frame;
        };

        frame2
    }
}

fn modify_settings_js(tpl: Vec<u8>) -> Vec<u8> {
    const TARGET: &str = "/* SETTINGS_LAST_MONITOR_FIELD */";
    const JAVASCRIPT: &str = "
		monitorFields.thumbScale = fieldTemplate.select(
			\"Thumbnail scale\",
			[\"full\", \"half\", \"third\", \"quarter\", \"sixth\", \"eighth\"],
			\"full\",
		);
    ";

    let tpl = String::from_utf8(tpl).expect("template should be valid utf8");
    let tpl = tpl.replace(TARGET, &(JAVASCRIPT.to_owned() + TARGET));
    tpl.as_bytes().to_owned()
}
