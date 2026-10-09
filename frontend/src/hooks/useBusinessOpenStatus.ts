"use client";

import { useEffect, useMemo, useState } from "react";

export interface BusinessOperatingHour {
  day_of_week: number;
  open_time: string;
  close_time: string;
  kitchen_close_time?: string | null;
  is_closed: boolean;
}

export interface BusinessOperatingException {
  exception_date: string; // YYYY-MM-DD
  open_time?: string | null;
  close_time?: string | null;
  kitchen_close_time?: string | null;
  is_closed: boolean;
}

export interface BusinessOpenStatus {
  isOpen: boolean;
  businessDate: string;
  businessTime: string;
}

const weekdayMap: Record<string, number> = {
  Sun: 0,
  Mon: 1,
  Tue: 2,
  Wed: 3,
  Thu: 4,
  Fri: 5,
  Sat: 6,
};

const parseClockMinutes = (value?: string | null): number | null => {
  if (!value) {
    return null;
  }

  const [hour, minute] = value.split(":").map(Number);
  if (Number.isNaN(hour) || Number.isNaN(minute)) {
    return null;
  }

  return hour * 60 + minute;
};

const getBusinessParts = (date: Date, timezone?: string) => {
  const formatter = new Intl.DateTimeFormat("en-US", {
    timeZone: timezone || "UTC",
    weekday: "short",
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  });

  const parts = formatter.formatToParts(date);
  const lookup = Object.fromEntries(
    parts
      .filter((part) => part.type !== "literal")
      .map((part) => [part.type, part.value]),
  ) as Record<string, string>;

  return {
    weekday: weekdayMap[lookup.weekday] ?? date.getUTCDay(),
    minutes:
      (Number.parseInt(lookup.hour || "0", 10) * 60) +
      Number.parseInt(lookup.minute || "0", 10),
    businessDate: `${lookup.year}-${lookup.month}-${lookup.day}`,
    businessTime: `${lookup.hour}:${lookup.minute}`,
  };
};

/** Effective dining close: kitchen close when set, otherwise door close. */
const effectiveCloseMinutes = (hours: {
  close_time: string;
  kitchen_close_time?: string | null;
}): number | null => {
  const kitchen = parseClockMinutes(hours.kitchen_close_time);
  if (kitchen !== null) {
    return kitchen;
  }
  return parseClockMinutes(hours.close_time);
};

export const getBusinessOpenStatus = (
  operatingHours: BusinessOperatingHour[],
  timezone?: string,
  now: Date = new Date(),
  exceptions: BusinessOperatingException[] = [],
): BusinessOpenStatus => {
  const parts = getBusinessParts(now, timezone);

  const todaysException = exceptions.find(
    (row) => row.exception_date === parts.businessDate,
  );
  if (todaysException?.is_closed) {
    return {
      isOpen: false,
      businessDate: parts.businessDate,
      businessTime: parts.businessTime,
    };
  }

  const hoursForToday: BusinessOperatingHour[] = todaysException && !todaysException.is_closed
    ? [
        {
          day_of_week: parts.weekday,
          open_time: todaysException.open_time || "00:00",
          close_time: todaysException.close_time || "23:59",
          kitchen_close_time: todaysException.kitchen_close_time,
          is_closed: false,
        },
      ]
    : operatingHours.filter((hours) => hours.day_of_week === parts.weekday);

  const previousDayHours = operatingHours.filter(
    (hours) => hours.day_of_week === (parts.weekday + 6) % 7,
  );

  const isWithinHours = (hours?: BusinessOperatingHour | null) => {
    if (!hours || hours.is_closed) {
      return false;
    }

    const openMinutes = parseClockMinutes(hours.open_time);
    const closeMinutes = effectiveCloseMinutes(hours);
    if (openMinutes === null || closeMinutes === null) {
      return false;
    }

    if (closeMinutes > openMinutes) {
      return parts.minutes >= openMinutes && parts.minutes < closeMinutes;
    }

    return parts.minutes >= openMinutes || parts.minutes < closeMinutes;
  };

  const isOpenToday = hoursForToday.some((h) => isWithinHours(h));
  const isOpenFromPreviousOvernight = previousDayHours.some((previous) => {
    if (previous.is_closed) return false;
    const previousOpen = parseClockMinutes(previous.open_time);
    const previousClose = effectiveCloseMinutes(previous);
    if (previousOpen === null || previousClose === null) {
      return false;
    }
    return previousClose <= previousOpen && parts.minutes < previousClose;
  });

  return {
    isOpen: Boolean(isOpenToday || isOpenFromPreviousOvernight),
    businessDate: parts.businessDate,
    businessTime: parts.businessTime,
  };
};

export function useBusinessOpenStatus(
  operatingHours: BusinessOperatingHour[],
  timezone?: string,
  exceptions: BusinessOperatingException[] = [],
) {
  const [now, setNow] = useState(() => new Date());

  useEffect(() => {
    const interval = window.setInterval(() => {
      setNow(new Date());
    }, 60_000);

    return () => window.clearInterval(interval);
  }, []);

  return useMemo(
    () => getBusinessOpenStatus(operatingHours, timezone, now, exceptions),
    [now, operatingHours, timezone, exceptions],
  );
}
