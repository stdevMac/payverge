export type LanguageDraft = {
  selected: string[];
  defaultLanguage: string;
};

export type LanguageDraftResult = LanguageDraft & {
  needsDefaultChoice: boolean;
  requiresRelabelConfirm: boolean;
};

export type BusinessLanguagesPayload = {
  language_codes: string[];
  default_code: string;
};

function unique(values: string[]): string[] {
  return [...new Set(values.filter(Boolean))];
}

export function applyLockedLanguageDraft(
  current: LanguageDraft,
  nextDefault: string,
): LanguageDraftResult {
  const next = nextDefault.trim();
  if (!next) {
    return {
      ...current,
      needsDefaultChoice: !current.defaultLanguage,
      requiresRelabelConfirm: false,
    };
  }

  const requiresRelabelConfirm = current.defaultLanguage !== next;
  if (current.selected.length <= 1) {
    return {
      selected: [next],
      defaultLanguage: next,
      needsDefaultChoice: false,
      requiresRelabelConfirm,
    };
  }

  const selected = current.selected.includes(next)
    ? current.selected
    : [...current.selected, next];
  return {
    selected,
    defaultLanguage: next,
    needsDefaultChoice: false,
    requiresRelabelConfirm,
  };
}

export function applyUnlockedLanguageDraft(
  current: LanguageDraft,
  nextSelected: string[],
): LanguageDraftResult {
  const selected = unique(nextSelected);
  if (selected.length === 0) {
    return {
      selected: current.selected,
      defaultLanguage: current.defaultLanguage,
      needsDefaultChoice: false,
      requiresRelabelConfirm: false,
    };
  }

  if (selected.includes(current.defaultLanguage)) {
    return {
      selected,
      defaultLanguage: current.defaultLanguage,
      needsDefaultChoice: false,
      requiresRelabelConfirm: false,
    };
  }

  return {
    selected,
    defaultLanguage: "",
    needsDefaultChoice: true,
    requiresRelabelConfirm: false,
  };
}

export function applyDefaultLanguageDraft(
  current: LanguageDraft,
  nextDefault: string,
): LanguageDraftResult {
  if (!current.selected.includes(nextDefault)) {
    return {
      ...current,
      needsDefaultChoice: true,
      requiresRelabelConfirm: false,
    };
  }

  return {
    selected: current.selected,
    defaultLanguage: nextDefault,
    needsDefaultChoice: false,
    requiresRelabelConfirm: false,
  };
}

export function canSaveLanguageDraft(draft: LanguageDraft): boolean {
  return (
    draft.selected.length > 0 &&
    Boolean(draft.defaultLanguage) &&
    draft.selected.includes(draft.defaultLanguage)
  );
}

export function buildBusinessLanguagesPayload(
  draft: LanguageDraft,
): BusinessLanguagesPayload {
  return {
    language_codes: draft.selected,
    default_code: draft.defaultLanguage,
  };
}

export function languageDraftsDiffer(
  a: LanguageDraft,
  b: LanguageDraft,
): boolean {
  const left = [...a.selected].sort();
  const right = [...b.selected].sort();
  return (
    JSON.stringify(left) !== JSON.stringify(right) ||
    a.defaultLanguage !== b.defaultLanguage
  );
}
