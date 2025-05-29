// SPDX-License-Identifier: GPL-2.0-or-later

use common::{Flags, monitor::ArcMonitorManager};
use log::Logger;
use monitor_groups::ArcMonitorGroups;
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
        use upon::Value;

        make_ascii_titlecase(&mut current_page);
        let log_sources_json = serde_json::to_string(&self.logger.sources())
            .expect("Vec<String> serialization to never fail");

        let monitors_json = if is_admin {
            serde_json::to_string(&self.monitor_manager.monitor_configs().await)
                .expect("serialization to never fail")
        } else {
            String::new()
        };
        let monitors_info_json =
            serde_json::to_string(&self.monitor_manager.monitors_info().await?)
                .expect("serialization to never fail");

        let monitor_groups_json = serde_json::to_string(&self.monitor_groups.get().await)
            .expect("serialization to never fail");

        let flags_json = serde_json::to_string(&self.flags).expect("serialization to never fail");

        Some(HashMap::from([
            ("current_page", Value::String(current_page)),
            ("csrf_token", Value::String(csrf_token)),
            ("flags", Value::String(flags_json)),
            ("is_admin", Value::Bool(is_admin)),
            ("log_sources_json", Value::String(log_sources_json)),
            ("monitor_groups_json", Value::String(monitor_groups_json)),
            ("monitors_json", Value::String(monitors_json)),
            ("monitors_info_json", Value::String(monitors_info_json)),
            ("tz", Value::String(self.time_zone.clone())),
        ]))
    }
}

/// Make the first character in a string uppercase.
fn make_ascii_titlecase(s: &mut str) {
    if let Some(r) = s.get_mut(0..1) {
        r.make_ascii_uppercase();
    }
}
