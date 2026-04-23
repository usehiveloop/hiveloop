use super::super::*;
use chrono::Utc;

#[test]
fn test_date_tracker_no_change_same_day() {
    let now = Utc::now();
    let mut tracker = DateTracker::with_date(now);

    // Should return None when no time has passed
    let result = tracker.check_date_change();
    assert!(result.is_none());
}

#[test]
fn test_date_tracker_detects_change_next_day() {
    // Create a tracker for yesterday
    let yesterday = Utc::now() - chrono::Duration::days(1);
    let mut tracker = DateTracker::with_date(yesterday);

    // Check should detect the 1-day change
    let result = tracker.check_date_change();
    assert!(result.is_some());

    let message = result.unwrap();
    assert!(message.contains("<system-reminder>"));
    assert!(message.contains("# Date Change"));
    assert!(message.contains("The date has changed"));
    assert!(message.contains("(was "));
    assert!(message.contains("</system-reminder>"));
}

#[test]
fn test_date_tracker_detects_multi_day_change() {
    // Create a tracker for 3 days ago
    let three_days_ago = Utc::now() - chrono::Duration::days(3);
    let mut tracker = DateTracker::with_date(three_days_ago);

    let result = tracker.check_date_change();
    assert!(result.is_some());

    let message = result.unwrap();
    assert!(message.contains("3 days"));
    assert!(message.contains("The date has changed by"));
}

#[test]
fn test_date_tracker_updates_internal_date() {
    let yesterday = Utc::now() - chrono::Duration::days(1);
    let mut tracker = DateTracker::with_date(yesterday);

    let old_date = tracker.current_date();
    tracker.check_date_change();
    let new_date = tracker.current_date();

    // The tracker should have updated to today
    assert_ne!(old_date.date_naive(), new_date.date_naive());
    assert_eq!(new_date.date_naive(), Utc::now().date_naive());
}

#[test]
fn test_date_tracker_no_false_positive_after_update() {
    let yesterday = Utc::now() - chrono::Duration::days(1);
    let mut tracker = DateTracker::with_date(yesterday);

    // First check detects change
    assert!(tracker.check_date_change().is_some());

    // Second check (same day) should not detect change
    assert!(tracker.check_date_change().is_none());
}
