/**
 * Formats an ISO 8601 date string into a user-friendly relative or calendar time representation.
 */
export function formatRelativeTime(dateString: string, now: Date = new Date()): string {
  const date = new Date(dateString);
  const timestamp = date.getTime();

  if (Number.isNaN(timestamp)) {
    return 'Unknown time';
  }

  const diffSeconds = Math.max(0, Math.floor((now.getTime() - timestamp) / 1000));

  if (diffSeconds < 60) {
    return 'Just now';
  }

  const diffMinutes = Math.floor(diffSeconds / 60);
  if (diffMinutes === 1) {
    return '1 minute ago';
  }
  if (diffMinutes < 60) {
    return `${diffMinutes} minutes ago`;
  }

  const diffHours = Math.floor(diffMinutes / 60);
  if (diffHours === 1) {
    return '1 hour ago';
  }
  if (diffHours < 24) {
    return `${diffHours} hours ago`;
  }

  const diffDays = Math.floor(diffHours / 24);
  if (diffDays === 1) {
    return 'Yesterday';
  }
  if (diffDays < 30) {
    return `${diffDays} days ago`;
  }

  // Format as readable date YYYY-MM-DD
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, '0');
  const day = String(date.getDate()).padStart(2, '0');
  return `${year}-${month}-${day}`;
}
