const DATE_TIME_FORMATTER = new Intl.DateTimeFormat("en", {
  dateStyle: "medium",
  timeStyle: "short",
});

export function formatExecutionDateTime(value: string | undefined) {
  if (!value) {
    return "—";
  }

  const date = new Date(value);

  if (Number.isNaN(date.getTime())) {
    return value;
  }

  return DATE_TIME_FORMATTER.format(date);
}

export function shortenExecutionId(value: string) {
  if (value.length <= 16) {
    return value;
  }

  return `${value.slice(0, 8)}…${value.slice(-6)}`;
}

export function formatExecutionDuration(
  startValue: string | undefined,
  endValue: string | undefined,
) {
  if (!startValue) {
    return "—";
  }

  const start = new Date(startValue);

  if (Number.isNaN(start.getTime())) {
    return "—";
  }

  const end = endValue ? new Date(endValue) : new Date();

  if (Number.isNaN(end.getTime())) {
    return "—";
  }

  const durationMs = Math.max(0, end.getTime() - start.getTime());

  if (durationMs < 1_000) {
    return `${durationMs} ms`;
  }

  const totalSeconds = Math.floor(durationMs / 1_000);

  if (totalSeconds < 60) {
    return `${totalSeconds} sec`;
  }

  const totalMinutes = Math.floor(totalSeconds / 60);

  const remainingSeconds = totalSeconds % 60;

  if (totalMinutes < 60) {
    return `${totalMinutes} min ${remainingSeconds} sec`;
  }

  const totalHours = Math.floor(totalMinutes / 60);

  const remainingMinutes = totalMinutes % 60;

  if (totalHours < 24) {
    return `${totalHours} hr ${remainingMinutes} min`;
  }

  const totalDays = Math.floor(totalHours / 24);

  const remainingHours = totalHours % 24;

  return `${totalDays} d ${remainingHours} hr`;
}
