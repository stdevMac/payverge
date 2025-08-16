export const formatDate = (dateString: string | number | Date): string => {
    if (!dateString) return "-";

    try {
        const date = new Date(dateString);

        // Check if date is valid
        if (isNaN(date.getTime())) {
            return "-";
        }

        return new Intl.DateTimeFormat("en-US", {
            year: "numeric",
            month: "short",
            day: "numeric",
        }).format(date);
    } catch (error) {
        return "-";
    }
};
