// SPDX-License-Identifier: GPL-2.0-or-later

use crate::QueryResult;
use crate::VodQuery;
use std::{collections::HashMap, sync::Arc};
use tokio::sync::Mutex;

// Caches the n most recent vod readers.
#[derive(Clone)]
#[allow(clippy::module_name_repetitions)]
pub struct VodCache(Arc<Mutex<State>>);

struct State {
    items: HashMap<VodQuery, CacheItem>,
    age: usize,

    max_size: usize,
}

struct CacheItem {
    age: usize,
    data: Arc<QueryResult>,
}

const VOD_CACHE_SIZE: usize = 10;

impl VodCache {
    #[must_use]
    pub fn new() -> Self {
        Self::with_capacity(VOD_CACHE_SIZE)
    }

    fn with_capacity(max_size: usize) -> Self {
        Self(Arc::new(Mutex::new(State {
            items: HashMap::new(),
            age: 0,
            max_size,
        })))
    }

    pub(crate) async fn add(&self, key: VodQuery, res: Arc<QueryResult>) {
        self.0.lock().await.add(key, res);
    }

    pub(crate) async fn get(&self, key: &VodQuery) -> Option<Arc<QueryResult>> {
        self.0.lock().await.get(key)
    }
}

impl Default for VodCache {
    fn default() -> Self {
        Self::new()
    }
}

impl State {
    fn add(&mut self, key: VodQuery, res: Arc<QueryResult>) {
        // Ignore duplicate keys.
        if self.items.contains_key(&key) {
            return;
        }

        self.age += 1;

        if self.items.len() >= self.max_size {
            // Delete the oldest item.
            let (key, _) = self
                .items
                .iter()
                .min_by_key(|(_, v)| v.age)
                .expect("len > max_size");
            self.items.remove(&key.to_owned());
        }

        self.items.insert(
            key,
            CacheItem {
                age: self.age,
                data: res,
            },
        );
    }

    fn get(&mut self, key: &VodQuery) -> Option<Arc<QueryResult>> {
        for (item_key, item) in &mut self.items {
            if item_key == key {
                self.age += 1;
                item.age = self.age;
                return Some(item.data.clone());
            }
        }
        None
    }
}

#[cfg(test)]
#[allow(clippy::unwrap_used)]
mod tests {
    use super::*;
    use common::time::UnixNano;

    fn key(v: u32) -> VodQuery {
        VodQuery {
            monitor_id: "x".to_owned().try_into().unwrap(),
            start: UnixNano::new(0),
            end: UnixNano::new(0),
            cache_id: v,
        }
    }

    fn empty() -> Arc<QueryResult> {
        Arc::new(QueryResult {
            meta: Vec::new(),
            meta_size: 0,
            size: 0,
            recs: Vec::new(),
        })
    }

    #[tokio::test]
    async fn test_video_reader_cache() {
        let cache = VodCache::with_capacity(3);

        // Fill cache.
        cache.add(key(1), empty()).await;
        cache.add(key(2), empty()).await;
        cache.add(key(3), empty()).await;

        // Add item and check if 1 was removed.
        cache.add(key(4), empty()).await;
        assert!(cache.get(&key(1)).await.is_none());

        // Get "B" to make it the newest item.
        cache.get(&key(2)).await;

        // Add item and check if "C" was removed instead of "B".
        let e = Arc::new(QueryResult {
            meta: Vec::new(),
            meta_size: 0,
            size: 100,
            recs: Vec::new(),
        });
        cache.add(key(5), e.clone()).await;
        assert!(cache.get(&key(3)).await.is_none());

        // Add item and check if "D" was removed instead of "B".
        cache.add(key(6), empty()).await;
        assert!(cache.get(&key(4)).await.is_none());

        // Add item and check if "B" was removed.
        cache.add(key(7), empty()).await;
        assert!(cache.get(&key(2)).await.is_none());

        // Check if duplicate keys are ignored.
        cache.add(key(7), empty()).await;
        let e2 = cache.get(&key(5)).await.unwrap();
        assert_eq!(e, e2);
    }
}
