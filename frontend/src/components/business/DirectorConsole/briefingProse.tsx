import React from "react";

// Shared helpers for the Sage briefing's editorial prose.
//
// The briefing is NOT a KPI card grid — it reads like a GM's brief, with the
// living numbers woven into sentences as emphasized inline figures. `Figure`
// is that emphasis (a subtle charcoal weight bump, no box), and `weave` splits
// a localized template on its `{token}` placeholders so the component can drop
// pre-formatted figure nodes into the surrounding prose without turning each
// number into its own card.

/** An emphasized inline figure inside the briefing prose. */
export function Figure({ children }: { children: React.ReactNode }) {
  return <span className="font-medium text-ink-900">{children}</span>;
}

/**
 * Split `template` on its `{token}` placeholders and substitute the matching
 * React node from `parts`, leaving all surrounding text as plain prose. Tokens
 * with no entry in `parts` are rendered as their literal `{token}` text (a
 * visible signal of a missing figure rather than a silent drop).
 */
export function weave(
  template: string,
  parts: Record<string, React.ReactNode>,
): React.ReactNode[] {
  return template.split(/(\{[a-zA-Z_]+\})/g).map((segment, index) => {
    const match = segment.match(/^\{([a-zA-Z_]+)\}$/);
    if (match && parts[match[1]] !== undefined) {
      return <React.Fragment key={index}>{parts[match[1]]}</React.Fragment>;
    }
    return <React.Fragment key={index}>{segment}</React.Fragment>;
  });
}
