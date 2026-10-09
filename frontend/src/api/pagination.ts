export interface PaginationOptions {
  page?: number;
  pageSize?: number;
}

interface PaginationMetadata {
  total: number;
  page: number;
  page_size: number;
  total_pages: number;
}

export interface PaginatedResponse<T> extends Partial<PaginationMetadata> {
  [key: string]: unknown;
}

export interface FetchAllPagesResult<T> {
  items: T[];
  metadata: PaginationMetadata;
  capped: boolean;
  warning?: string;
}

export interface FetchAllPagesOptions {
  pageSize?: number;
  maxPages?: number;
  maxRows?: number;
}

const DEFAULT_PAGE_SIZE = 100;
const DEFAULT_MAX_PAGES = 10;
const DEFAULT_MAX_ROWS = 1000;

const normalizePositiveInt = (value: unknown, fallback: number): number => {
  const parsed = Number(value);
  if (!Number.isFinite(parsed) || parsed < 1) {
    return fallback;
  }
  return Math.floor(parsed);
};

export async function fetchAllPages<T>(
  fetchPage: (page: number, pageSize: number) => Promise<PaginatedResponse<T>>,
  selectItems: (response: PaginatedResponse<T>) => T[] | undefined,
  options: FetchAllPagesOptions = {},
): Promise<FetchAllPagesResult<T>> {
  const pageSize = normalizePositiveInt(options.pageSize, DEFAULT_PAGE_SIZE);
  const maxPages = normalizePositiveInt(options.maxPages, DEFAULT_MAX_PAGES);
  const maxRows = normalizePositiveInt(options.maxRows, DEFAULT_MAX_ROWS);
  const items: T[] = [];
  let total = 0;
  let totalPages = 1;
  let page = 1;
  let lastFetchedPage = 1;
  let responsePageSize = pageSize;
  let capped = false;
  let warning: string | undefined;

  while (page <= totalPages) {
    if (page > maxPages || items.length >= maxRows) {
      capped = true;
      warning = "live_data_cap_reached";
      break;
    }

    const response = await fetchPage(page, pageSize);
    lastFetchedPage = page;
    const pageItems = selectItems(response) ?? [];
    items.push(...pageItems);

    total = normalizePositiveInt(response.total, items.length);
    totalPages = normalizePositiveInt(response.total_pages, page);
    responsePageSize = normalizePositiveInt(response.page_size, pageSize);

    if (pageItems.length === 0 || page >= totalPages) {
      break;
    }
    page += 1;
  }

  const trimmedItems = items.slice(0, maxRows);
  if (trimmedItems.length < items.length) {
    capped = true;
    warning = "live_data_cap_reached";
  }

  return {
    items: trimmedItems,
    metadata: {
      total,
      page: Math.min(lastFetchedPage, totalPages),
      page_size: normalizePositiveInt(responsePageSize, pageSize),
      total_pages: totalPages,
    },
    capped,
    warning,
  };
}
