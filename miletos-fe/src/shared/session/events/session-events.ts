export const SESSION_UNAUTHORIZED_EVENT = "miletos:session-unauthorized";

export const dispatchUnauthorizedEvent = () => {
  if (typeof window !== "undefined") {
    window.dispatchEvent(new Event(SESSION_UNAUTHORIZED_EVENT));
  }
};
