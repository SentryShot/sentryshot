// SPDX-License-Identifier: GPL-2.0-or-later

use common::monitor::ArcMonitorManager;
use log::Logger;
use monitor_groups::ArcMonitorGroups;
use std::{collections::HashMap, sync::Arc};

pub struct Templater<'a> {
    logger: Arc<Logger>,
    monitor_manager: ArcMonitorManager,
    monitor_groups: ArcMonitorGroups,
    time_zone: String,

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
    ) -> HashMap<String, upon::Value> {
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
        let monitors_info_json = serde_json::to_string(&self.monitor_manager.monitors_info().await)
            .expect("serialization to never fail");

        let monitor_groups_json = serde_json::to_string(&self.monitor_groups.get().await)
            .expect("serialization to never fail");

        HashMap::from([
            ("groups_json".to_owned(), Value::String("{}".to_owned())),
            ("monitors_json".to_owned(), Value::String(monitors_json)),
            (
                "monitors_info_json".to_owned(),
                Value::String(monitors_info_json),
            ),
            (
                "monitor_groups_json".to_owned(),
                Value::String(monitor_groups_json),
            ),
            ("tz".to_owned(), Value::String(self.time_zone.clone())),
            (
                "log_sources_json".to_owned(),
                Value::String(log_sources_json),
            ),
            ("is_admin".to_owned(), Value::Bool(is_admin)),
            ("csrf_token".to_owned(), Value::String(csrf_token)),
            ("current_page".to_owned(), Value::String(current_page)),
        ])
    }
}

/// Make the first character in a string uppercase.
fn make_ascii_titlecase(s: &mut str) {
    if let Some(r) = s.get_mut(0..1) {
        r.make_ascii_uppercase();
    }
}
