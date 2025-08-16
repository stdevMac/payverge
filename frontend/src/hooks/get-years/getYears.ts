type YearObject = { key: string; label: string };

function getYears(): YearObject[] {
    const currentYear = new Date().getFullYear();
    const years: YearObject[] = [];

    for (let i = 0; i <= 5; i++) {
        const year = currentYear - i;
        years.push({ key: `${year}`, label: `${year}` });
    }

    return years;
}

export const years: YearObject[] = getYears();
