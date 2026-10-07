import React from "react";

/**
 * Placeholder grid that matches the MenuList layout so operators see the
 * shape of their menu loading in rather than a bare AI toolbar with no
 * categories underneath. Tokens stay on the ink palette per CLAUDE.md.
 */
export function MenuLoadingSkeleton() {
    return (
        <div
            data-testid="menu-loading-skeleton"
            className="space-y-8 animate-pulse"
            aria-busy="true"
            aria-live="polite"
        >
            {[0, 1, 2].map((cat) => (
                <div key={cat}>
                    <div className="h-6 w-48 bg-warm-100 rounded mb-2" />
                    <div className="h-4 w-72 bg-warm-100 rounded mb-4" />
                    <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
                        {[0, 1, 2, 3].map((item) => (
                            <div
                                key={item}
                                className="border border-warm-100 rounded-2xl overflow-hidden"
                            >
                                <div className="aspect-[5/4] bg-warm-100" />
                                <div className="p-3 space-y-2">
                                    <div className="h-4 w-3/4 bg-warm-100 rounded" />
                                    <div className="h-3 w-1/2 bg-warm-100 rounded" />
                                </div>
                            </div>
                        ))}
                    </div>
                </div>
            ))}
        </div>
    );
}
