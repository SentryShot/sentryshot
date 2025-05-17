// SPDX-License-Identifier: GPL-2.0-or-later

mod config;

use async_trait::async_trait;
use common::{
    monitor::MonitorConfig, ArcLogger, Event, EventSource, Label, LogEntry, LogLevel, LogSource,
    MonitorId, MonitorName,
};
use config::Config;
use jiff::Timestamp;
use plugin::{Application, Plugin, PreLoadPlugin};
use rumqttc::{ClientError, MqttOptions, QoS};
use serde::Serialize;
use std::{sync::Arc, time::Duration};
use thiserror::Error;
use tokio::{runtime::Handle, sync::mpsc};
use tokio_util::sync::CancellationToken;

#[no_mangle]
pub extern "Rust" fn version() -> String {
    plugin::get_version()
}
#[no_mangle]
pub extern "Rust" fn pre_load() -> Box<dyn PreLoadPlugin> {
    Box::new(PreLoadMqtt)
}
struct PreLoadMqtt;
impl PreLoadPlugin for PreLoadMqtt {
    fn add_log_source(&self) -> Option<LogSource> {
        #[allow(clippy::unwrap_used)]
        Some("mqtt".try_into().unwrap())
    }
}
#[no_mangle]
pub extern "Rust" fn load(app: &dyn Application) -> Arc<dyn Plugin> {
    Arc::new(MqttPlugin::new(
        app.rt_handle(),
        app.token(),
        app.logger(),
        app.env().raw(),
    ))
}

struct MqttPlugin {
    logger: ArcLogger,
    publisher: Publisher,
}

#[async_trait]
impl Plugin for MqttPlugin {
    async fn on_event(&self, event: Event, config: MonitorConfig) {
        if let Err(e) = self
            .publisher
            .publish(event, config.id(), config.name())
            .await
        {
            self.logger.log(LogEntry::new(
                LogLevel::Error,
                "mqtt",
                None,
                format!("publish: {e}"),
            ));
        };
    }
}

impl MqttPlugin {
    fn new(
        rt_handle: Handle,
        token: CancellationToken,
        logger: ArcLogger,
        raw_env_config: &str,
    ) -> Self {
        let config = match raw_env_config.parse::<Config>() {
            Ok(v) => v,
            Err(e) => {
                eprintln!("failed to parse mqtt config: {e}");
                std::process::exit(1);
            }
        };
        let options = MqttOptions::new("sentryshot", config.host, config.port);
        let (client, mut event_loop) = rumqttc::Client::new(options, 1);

        let (tx, mut rx) = mpsc::channel(64);

        // This doesn't exit until the app does.
        // Making this `spawn_blocking` causes the app to never exit for some reason.
        std::thread::spawn(move || {
            let mut last_connection_refused_message = std::time::Instant::now();

            while let Ok(res) = event_loop.recv() {
                match &res {
                    Err(rumqttc::ConnectionError::ConnectionRefused(_)) => {
                        if last_connection_refused_message.elapsed().as_secs() > 3 {
                            last_connection_refused_message = std::time::Instant::now();
                        } else {
                            std::thread::sleep(Duration::from_millis(10));
                            continue;
                        }
                    }
                    Err(rumqttc::ConnectionError::Io(e)) => {
                        if let std::io::ErrorKind::ConnectionRefused = e.kind() {
                            if last_connection_refused_message.elapsed().as_secs() > 3 {
                                last_connection_refused_message = std::time::Instant::now();
                            } else {
                                std::thread::sleep(Duration::from_millis(10));
                                continue;
                            }
                        };
                    }
                    _ => {}
                };
                if tx.blocking_send(res).is_err() {
                    // Receiver was dropped.
                    return;
                }
            }
        });

        // Process events.
        let logger2 = logger.clone();
        rt_handle.spawn(async move {
            loop {
                tokio::select! {
                    () = token.cancelled() => {
                        return
                    }
                    res = rx.recv() => {
                        let Some(res) = res else {
                            // The event loop somehow exited.
                            return
                        };
                        match res {
                            Ok(v) => logger2.log(LogEntry::new(
                                         LogLevel::Debug,
                                         "mqtt",
                                         None,
                                         format!("{v:?}"),
                                     )),
                            Err(e) => logger2.log(LogEntry::new(
                                          LogLevel::Error,
                                          "mqtt",
                                          None,
                                          e.to_string(),
                                      )),
                        };
                    }
                }
            }
        });

        let publisher = Publisher::new(rt_handle, client);
        Self { logger, publisher }
    }
}

#[derive(Debug, Error)]
enum PublishError {
    #[error(transparent)]
    Serialize(#[from] serde_json::error::Error),

    #[error(transparent)]
    Client(#[from] ClientError),
}

struct Publisher {
    rt_handle: Handle,
    client: rumqttc::Client,
}

impl Publisher {
    fn new(rt_handle: Handle, client: rumqttc::Client) -> Self {
        Self { rt_handle, client }
    }

    async fn publish(
        &self,
        event: Event,
        monitor_id: &MonitorId,
        monitor_name: &MonitorName,
    ) -> Result<(), PublishError> {
        for event in parse_event(event, monitor_id, monitor_name) {
            let msg = serde_json::to_string_pretty(&event)?;
            let client = self.client.clone();
            // The spawn_blocking may be unnecessary.
            self.rt_handle
                .spawn_blocking(move || -> Result<(), ClientError> {
                    client.try_publish(
                        format!("sentryshot/events/{}", event.source),
                        QoS::ExactlyOnce,
                        false,
                        msg,
                    )
                })
                .await
                .expect("join")?;
        }
        Ok(())
    }
}

#[derive(Serialize)]
struct MqttEvent {
    #[serde(rename = "monitorID")]
    monitor_id: MonitorId,
    #[serde(rename = "monitorName")]
    monitor_name: MonitorName,

    label: Label,
    score: f32,

    time: Timestamp,
    source: EventSource,
}

fn parse_event(
    mut event: Event,
    monitor_id: &MonitorId,
    monitor_name: &MonitorName,
) -> Vec<MqttEvent> {
    let Some(source) = event.source.take() else {
        return Vec::new();
    };
    let mut events = Vec::new();
    for d in event.detections {
        events.push(MqttEvent {
            monitor_id: monitor_id.clone(),
            monitor_name: monitor_name.clone(),
            label: d.label,
            score: d.score,
            time: event.time.into(),
            source: source.clone(),
        });
    }
    events
}

#[cfg(test)]
#[allow(clippy::unwrap_used)]
mod tests {
    use super::*;
    use common::{
        time::{Duration, UnixNano},
        Detection, Region,
    };
    use pretty_assertions::assert_eq;
    use rumqttc::Publish;
    use serde_json::json;

    #[tokio::test]
    async fn test_mqtt() {
        let (tx, rx) = flume::bounded(1);
        let recv = |topic: &str| -> serde_json::Value {
            match rx.recv().unwrap() {
                rumqttc::Request::Publish(v) => {
                    let want = Publish {
                        dup: false,
                        qos: QoS::ExactlyOnce,
                        retain: false,
                        topic: topic.to_owned(),
                        pkid: 0,
                        payload: v.payload.clone(),
                    };
                    assert_eq!(want, v);
                    serde_json::from_slice(&v.payload).unwrap()
                }
                _ => panic!(""),
            }
        };

        let publisher = Publisher::new(Handle::current(), rumqttc::Client::from_sender(tx));
        let m_id = |monitor_id: &str| monitor_id.to_owned().try_into().unwrap();
        let m_name = |monitor_name: &str| monitor_name.to_owned().try_into().unwrap();

        let e1 = test_event(
            1,
            "src1",
            vec![Detection {
                label: "person".to_owned().try_into().unwrap(),
                score: 12.34,
                region: Region::default(),
            }],
        );
        publisher
            .publish(e1, &m_id("id1"), &m_name("name1"))
            .await
            .unwrap();
        assert_eq!(
            json!({
                "monitorID": "id1",
                "monitorName": "name1",
                "label": "person",
                "score": 12.34,
                "time": "1970-01-01T00:00:00.000000001Z",
                "source": "src1"
            }),
            recv("sentryshot/events/src1")
        );

        let e2 = test_event(
            2,
            "src2",
            vec![Detection {
                label: "person".to_owned().try_into().unwrap(),
                score: 56.78,
                region: Region::default(),
            }],
        );
        publisher
            .publish(e2, &m_id("id2"), &m_name("name2"))
            .await
            .unwrap();
        assert_eq!(
            json!({
                "monitorID": "id2",
                "monitorName": "name2",
                "label": "person",
                "score": 56.78,
                "time": "1970-01-01T00:00:00.000000002Z",
                "source": "src2"
            }),
            recv("sentryshot/events/src2")
        );
    }

    fn test_event(time: i64, source: &str, detections: Vec<Detection>) -> Event {
        Event {
            time: UnixNano::new(time),
            duration: Duration::new(0),
            detections,
            source: Some(source.to_owned().try_into().unwrap()),
        }
    }
}
