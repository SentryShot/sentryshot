// SPDX-License-Identifier: GPL-2.0-or-later

use common::{Flags, monitor::ArcMonitorManager};
use log::Logger;
use monitor_groups::ArcMonitorGroups;
use serde_json::Value;
use std::{collections::HashMap, sync::Arc};

pub struct Templater<'a> {
    logger: Arc<Logger>,
    monitor_manager: ArcMonitorManager,
    monitor_groups: ArcMonitorGroups,
    time_zone: String,
    flags: Flags,

    engine: upon::Engine<'a>,
}

impl<'a> Templater<'a> {
    #[must_use]
    pub fn new(
        logger: Arc<log::Logger>,
        monitor_manager: ArcMonitorManager,
        monitor_groups: ArcMonitorGroups,
        templates: HashMap<&'a str, String>,
        time_zone: String,
        flags: Flags,
    ) -> Self {
        let mut engine = upon::Engine::new();
        for (k, v) in templates {
            engine.add_template(k, v).expect("template should compile");
        }

        Self {
            logger,
            monitor_manager,
            monitor_groups,
            time_zone,
            flags,
            engine,
        }
    }

    #[must_use]
    pub fn logger(&self) -> &Arc<Logger> {
        &self.logger
    }

    #[must_use]
    pub fn get_template(&self, name: &str) -> Option<upon::TemplateRef> {
        self.engine.get_template(name)
    }

    pub async fn get_data(
        &self,
        mut current_page: String,
        is_admin: bool,
        csrf_token: String,
    ) -> Option<HashMap<&'static str, upon::Value>> {
        make_ascii_titlecase(&mut current_page);

        let mut ui_data = serde_json::Map::new();
        ui_data.insert(
            "currentPage".to_owned(),
            Value::String(current_page.clone()),
        );
        ui_data.insert("csrfToken".to_owned(), Value::String(csrf_token.clone()));
        ui_data.insert(
            "flags".to_owned(),
            serde_json::to_value(self.flags).expect("serialization to never fail"),
        );
        ui_data.insert("isAdmin".to_owned(), Value::Bool(is_admin));
        ui_data.insert("tz".to_owned(), Value::String(self.time_zone.clone()));
        ui_data.insert(
            "logSources".to_owned(),
            serde_json::to_value(self.logger.sources())
                .expect("Vec<String> serialization to never fail"),
        );
        ui_data.insert(
            "monitorGroups".to_owned(),
            serde_json::to_value(&self.monitor_groups.get().await)
                .expect("serialization to never fail"),
        );
        if is_admin {
            ui_data.insert(
                "monitors".to_owned(),
                serde_json::to_value(&self.monitor_manager.monitor_configs().await)
                    .expect("serialization to never fail"),
            );
        };
        ui_data.insert(
            "monitorsInfo".to_owned(),
            serde_json::to_value(&self.monitor_manager.monitors_info().await?)
                .expect("serialization to never fail"),
        );

        // ui_data plugin hook.

        let ui_data_json =
            serde_json::to_string(&ui_data).expect("Vec<String> serialization to never fail");

        Some(HashMap::from([
            ("current_page", upon::Value::String(current_page)),
            ("is_admin", upon::Value::Bool(is_admin)),
            ("ui_data", upon::Value::String(ui_data_json)),
        ]))
    }
}

/// Make the first character in a string uppercase.
fn make_ascii_titlecase(s: &mut str) {
    if let Some(r) = s.get_mut(0..1) {
        r.make_ascii_uppercase();
    }
}
