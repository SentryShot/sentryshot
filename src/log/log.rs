// SPDX-License-Identifier: GPL-2.0-or-later

pub mod log_db;
pub mod rev_buf_reader;
pub mod slow_poller;

use common::{ILogger, LogEntry, LogLevel, LogMessage, LogSource, MonitorId};
use serde::{Deserialize, Serialize};
use std::{
    fmt,
    ops::Deref,
    time::{SystemTime, UNIX_EPOCH},
};

use tokio::sync::broadcast;

/// Logger used everywhere across the application.
pub struct Logger {
    /// Internal logging feed.
    feed: broadcast::Sender<LogEntryWithTime>,

    sources: Vec<LogSource>,
}

impl Logger {
    /// Creates a new logger.
    #[must_use]
    #[allow(clippy::unwrap_used)]
    pub fn new(sources: Vec<LogSource>) -> Self {
        let (feed, _) = broadcast::channel(512);

        let mut sources = sources;
        sources.push("app".try_into().unwrap());
        sources.push("monitor".try_into().unwrap());
        sources.sort();

        Self { feed, sources }
    }

    /// Subscribes to the log feed and returns a channel that receives all log entries.
    #[must_use]
    pub fn subscribe(&self) -> broadcast::Receiver<LogEntryWithTime> {
        self.feed.subscribe()
    }

    #[must_use]
    pub fn sources(&self) -> &Vec<LogSource> {
        &self.sources
    }
}

impl Default for Logger {
    fn default() -> Self {
        Self::new(Vec::new())
    }
}

impl ILogger for Logger {
    /// Sends log entry to all subscribers. The timestamp is applied now.
    fn log(&self, log: LogEntry) {
        let log = LogEntryWithTime {
            level: log.level,
            source: log.source,
            monitor_id: log.monitor_id,
            message: log.message,
            time: UnixMicro::now(),
        };

        // Print to stdout.
        println!("{log}");

        // Only returns an error if there are no subscribers.
        self.feed.send(log).ok();
    }
}

/// Microseconds since the `UNIX_EPOCH`.
#[repr(transparent)]
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq, PartialOrd, Ord, Serialize, Deserialize)]
pub struct UnixMicro(u64);

impl UnixMicro {
    #[must_use]
    pub fn new(v: u64) -> Self {
        Self(v)
    }

    #[must_use]
    pub fn from_millis(ms: u32) -> Self {
        Self(u64::from(ms) * 1000)
    }

    #[must_use]
    pub fn from_secs(s: u16) -> Self {
        Self(u64::from(s) * 1_000_000)
    }

    /// Current time as `UnixMicro`.
    fn now() -> Self {
        UnixMicro(
            u64::try_from(
                SystemTime::now()
                    .duration_since(UNIX_EPOCH)
                    .expect("broken system clock")
                    .as_micros(),
            )
            .expect("really broken system clock"),
        )
    }

    #[must_use]
    pub fn checked_add(&self, rhs: Self) -> Option<Self> {
        Some(Self(self.0.checked_add(rhs.0)?))
    }

    #[must_use]
    pub fn checked_sub(&self, rhs: Self) -> Option<Self> {
        Some(Self(self.0.checked_sub(rhs.0)?))
    }
}

impl Deref for UnixMicro {
    type Target = u64;

    fn deref(&self) -> &Self::Target {
        &self.0
    }
}

/// Log entry with timestamp.
#[derive(Clone, Debug, PartialEq, Eq, Serialize)]
pub struct LogEntryWithTime {
    pub level: LogLevel,
    pub source: LogSource,

    /// Optional monitor ID if the message can be tied to a monitor.
    #[serde(rename = "monitorID", skip_serializing_if = "Option::is_none")]
    pub monitor_id: Option<MonitorId>,

    pub message: LogMessage,
    pub time: UnixMicro,
}

impl fmt::Display for LogEntryWithTime {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self.level {
            LogLevel::Error => write!(f, "[ERROR] ")?,
            LogLevel::Warning => write!(f, "[WARNING] ")?,
            LogLevel::Info => write!(f, "[INFO] ")?,
            LogLevel::Debug => write!(f, "[DEBUG] ")?,
        };

        if let Some(monitor_id) = &self.monitor_id {
            write!(f, "{monitor_id}: ")?;
        };

        let mut src_titel = self.source.to_string();
        make_ascii_titlecase(&mut src_titel);

        write!(f, "{}: {}", src_titel, self.message)?;

        Ok(())
    }
}

/// Make the first character in a string uppercase.
fn make_ascii_titlecase(s: &mut str) {
    if let Some(r) = s.get_mut(0..1) {
        r.make_ascii_uppercase();
    }
}

#[allow(clippy::needless_pass_by_value, clippy::unwrap_used)]
#[cfg(test)]
mod tests {
    use super::*;
    use common::{ParseLogMessageError, ParseLogSourceError};
    use pretty_assertions::assert_eq;
    use test_case::test_case;

    fn src(s: &'static str) -> LogSource {
        s.try_into().unwrap()
    }
    fn m_id(s: &str) -> MonitorId {
        s.to_owned().try_into().unwrap()
    }
    fn msg(s: &str) -> LogMessage {
        s.to_owned().try_into().unwrap()
    }

    #[tokio::test]
    async fn logger_messages() {
        let logger = Logger::new(Vec::new());
        let mut feed = logger.subscribe();

        logger.log(LogEntry {
            level: LogLevel::Info,
            source: src("s1"),
            monitor_id: Some(m_id("m1")),
            message: msg("1"),
        });
        logger.log(LogEntry {
            level: LogLevel::Warning,
            source: src("s2"),
            monitor_id: Some(m_id("m2")),
            message: msg("2"),
        });
        logger.log(LogEntry {
            level: LogLevel::Error,
            source: src("s3"),
            monitor_id: Some(m_id("m3")),
            message: msg("3"),
        });
        logger.log(LogEntry {
            level: LogLevel::Debug,
            source: src("s4"),
            monitor_id: Some(m_id("m4")),
            message: msg("4"),
        });

        let mut actual = vec![
            feed.recv().await.unwrap(),
            feed.recv().await.unwrap(),
            feed.recv().await.unwrap(),
            feed.recv().await.unwrap(),
        ];
        actual.iter_mut().for_each(|v| v.time = UnixMicro(0));

        let expected = vec![
            LogEntryWithTime {
                level: LogLevel::Info,
                source: src("s1"),
                monitor_id: Some(m_id("m1")),
                message: msg("1"),
                time: UnixMicro(0),
            },
            LogEntryWithTime {
                level: LogLevel::Warning,
                source: src("s2"),
                monitor_id: Some(m_id("m2")),
                message: msg("2"),
                time: UnixMicro(0),
            },
            LogEntryWithTime {
                level: LogLevel::Error,
                source: src("s3"),
                monitor_id: Some(m_id("m3")),
                message: msg("3"),
                time: UnixMicro(0),
            },
            LogEntryWithTime {
                level: LogLevel::Debug,
                source: src("s4"),
                monitor_id: Some(m_id("m4")),
                message: msg("4"),
                time: UnixMicro(0),
            },
        ];

        assert_eq!(actual, expected);
    }

    #[test_case("", ParseLogSourceError::Empty; "empty")]
    #[test_case("@",ParseLogSourceError::InvalidChars("@".to_owned()); "invalid_chars")]
    fn source_parse(input: &str, want: ParseLogSourceError) {
        assert_eq!(
            want,
            LogSource::try_from(input.to_owned()).expect_err("expected error")
        );
    }

    #[test_case("", ParseLogMessageError::Empty; "empty")]
    fn message_parse(input: &str, want: ParseLogMessageError) {
        assert_eq!(
            want,
            LogMessage::try_from(input.to_owned()).expect_err("expected error")
        );
    }
}
