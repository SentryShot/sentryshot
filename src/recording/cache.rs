use std::{
    collections::HashMap,
    path::{Path, PathBuf},
    sync::Arc,
};

use crate::video_reader::VideoMetadata;

// Caches the n most recent video readers.
#[allow(clippy::module_name_repetitions)]
pub struct VideoCache {
    items: HashMap<(PathBuf, u32), CacheItem>,
    age: usize,

    max_size: usize,
}

#[derive(Debug)]
struct CacheItem {
    age: usize,
    data: Arc<VideoMetadata>,
}

const VOD_CACHE_SIZE: usize = 10;

impl VideoCache {
    // NewVideoCache creates a video cache.
    #[must_use]
    pub fn new() -> Self {
        Self {
            items: HashMap::new(),
            age: 0,
            max_size: VOD_CACHE_SIZE,
        }
    }

    pub(crate) fn add(&mut self, key: (PathBuf, u32), video: Arc<VideoMetadata>) {
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
                data: video,
            },
        );
    }

    // Get item by key and update its age if it exists.
    pub(crate) fn get(&mut self, key: (&Path, u32)) -> Option<Arc<VideoMetadata>> {
        for (item_key, item) in &mut self.items {
            if item_key.0 == key.0 && item_key.1 == key.1 {
                self.age += 1;
                item.age = self.age;
                return Some(item.data.clone());
            }
        }
        None
    }
}

impl Default for VideoCache {
    fn default() -> Self {
        Self::new()
    }
}

#[cfg(test)]
#[allow(clippy::unwrap_used)]
mod tests {
    use std::time::UNIX_EPOCH;

    use super::*;

    fn empty_metadata() -> Arc<VideoMetadata> {
        Arc::new(VideoMetadata {
            buf: Vec::new(),
            mdat_size: 0,
            last_modified: UNIX_EPOCH,
        })
    }

    #[test]
    fn test_video_reader_cache() {
        let mut cache = VideoCache::new();
        cache.max_size = 3;

        // Fill cache.
        cache.add((PathBuf::from("A"), 0), empty_metadata());
        cache.add((PathBuf::from("B"), 1), empty_metadata());
        cache.add((PathBuf::from("C"), 0), empty_metadata());

        // Add item and check if "A" was removed.
        cache.add((PathBuf::from("D"), 0), empty_metadata());
        assert!(cache.get((Path::new("A"), 0)).is_none());

        // Get "B" to make it the newest item.
        cache.get((Path::new("B"), 1));

        // Add item and check if "C" was removed instead of "B".
        let e = Arc::new(VideoMetadata {
            buf: Vec::new(),
            mdat_size: 9999,
            last_modified: UNIX_EPOCH,
        });
        cache.add((PathBuf::from("E"), 0), e.clone());
        assert!(cache.get((Path::new("C"), 0)).is_none());

        // Add item and check if "D" was removed instead of "B".
        cache.add((PathBuf::from("F"), 0), empty_metadata());
        assert!(cache.get((Path::new("D"), 0)).is_none());

        // Add item and check if "B" was removed.
        cache.add((PathBuf::from("G"), 0), empty_metadata());
        assert!(cache.get((Path::new("B"), 1)).is_none());

        // Check if duplicate keys are ignored.
        cache.add((PathBuf::from("G"), 0), empty_metadata());
        let e2 = cache.get((Path::new("E"), 0)).unwrap();
        assert_eq!(e, e2);
    }
}
