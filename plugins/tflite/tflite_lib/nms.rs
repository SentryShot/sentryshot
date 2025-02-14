use crate::Detection;
use std::cmp::Ordering;

// https://learnopencv.com/non-maximum-suppression-theory-and-implementation-in-pytorch
pub fn non_max_suppression(mut detections: Vec<Detection>, iou_threshold: f32) -> Vec<Detection> {
    detections.sort_by(|a, b| a.score.partial_cmp(&b.score).unwrap_or(Ordering::Greater));
    let mut iterations = 0;
    let mut keep = Vec::new();
    while let Some(d) = detections.pop() {
        if iterations > 10000 {
            // This is a O(n²) algorithm in the very unlikely
            // worst case, so we need some type of limit.
            break;
        }
        detections.retain(|d2| {
            iterations += 1;
            iou(&d, d2) < iou_threshold
        });
        keep.push(d);
    }
    keep
}

// Intersection over union.
fn iou(a: &Detection, b: &Detection) -> f32 {
    let max_left = f32::max(a.left, b.left);
    let max_top = f32::max(a.top, b.top);
    let min_right = f32::min(a.right, b.right);
    let min_bottom = f32::min(a.bottom, b.bottom);

    let w = f32::max(0.0, min_right - max_left);
    let h = f32::max(0.0, min_bottom - max_top);

    let intersection_area = w * h;
    let union_area = a.area() + b.area() - intersection_area;

    intersection_area / union_area
}
