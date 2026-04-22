export function formatDateTime(value: string): string {
  if (!value) return 'n/a';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat(undefined, {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  }).format(date);
}

export function formatRelativeDateTime(value: string): string {
  if (!value) return 'n/a';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  const age = Date.now() - date.getTime();
  if (age < 0) return formatDateTime(value);
  if (age < 60_000) return 'just now';
  if (age < 3_600_000) {
    const minutes = Math.floor(age / 60_000);
    return `${minutes}m ago`;
  }
  if (age < 86_400_000) {
    const hours = Math.floor(age / 3_600_000);
    const minutes = Math.floor((age % 3_600_000) / 60_000);
    if (minutes === 0) return `${hours}h ago`;
    return `${hours}h ${minutes}m ago`;
  }
  return formatDateTime(value);
}
