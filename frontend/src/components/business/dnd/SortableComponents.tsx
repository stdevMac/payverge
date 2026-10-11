import React from "react";
import { useSortable } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { GripVertical } from "lucide-react";

export const SortableCategoryCard = ({
    id,
    children,
    className,
    data,
    ariaLabel,
}: any) => {
    const {
        attributes,
        listeners,
        setNodeRef,
        transform,
        transition,
        isDragging,
    } = useSortable({ id, data });

    const style = {
        transform: CSS.Translate.toString(transform),
        transition,
        zIndex: isDragging ? 50 : "auto",
        opacity: isDragging ? 0.5 : 1,
    };

    // The grip is absolutely positioned, so the wrapper MUST establish a
    // positioning context — without `relative` the grip anchors to a distant
    // ancestor and drifts off-card, leaving categories effectively undraggable.
    const wrapperClassName = ["relative", className].filter(Boolean).join(" ");

    return (
        <div ref={setNodeRef} style={style} className={wrapperClassName}>
            {/* Only the grip carries the dnd-kit listeners so the drag starts
                here while the rest of the card stays interactive. Mirrors the
                item handle in MenuItemCard (grip-as-handle). */}
            <div
                {...attributes}
                {...listeners}
                role="button"
                aria-label={ariaLabel}
                className="absolute left-1 top-5 z-20 cursor-grab rounded-lg p-1 text-ink-400 shadow-sm bg-white/90 transition hover:bg-warm-50 hover:text-ink-700"
            >
                <GripVertical className="w-5 h-5" />
            </div>
            <div className="pl-4">{children}</div>
        </div>
    );
};
