export interface Allergen {
    id: string;
    name: string;
    icon: string;
}

export interface DietaryTag {
    id: string;
    name: string;
    emoji: string;
    description: string;
}

export const ALLERGENS: Allergen[] = [
    { id: "celery", name: "allergenNames.celery", icon: "/images/allergens/celery.svg" },
    { id: "crustaceans", name: "allergenNames.crustaceans", icon: "/images/allergens/crustaceans.svg" },
    { id: "dairy", name: "allergenNames.dairy", icon: "/images/allergens/dairy.svg" },
    { id: "eggs", name: "allergenNames.eggs", icon: "/images/allergens/eggs.svg" },
    { id: "fish", name: "allergenNames.fish", icon: "/images/allergens/fish.svg" },
    { id: "gluten", name: "allergenNames.gluten", icon: "/images/allergens/gluten.svg" },
    { id: "lupin", name: "allergenNames.lupin", icon: "/images/allergens/lupin.svg" },
    { id: "mollusc", name: "allergenNames.mollusc", icon: "/images/allergens/mollusc.svg" },
    { id: "mustard", name: "allergenNames.mustard", icon: "/images/allergens/mustard.svg" },
    { id: "peanut", name: "allergenNames.peanut", icon: "/images/allergens/peanut.svg" },
    { id: "sesame", name: "allergenNames.sesame", icon: "/images/allergens/sesame.svg" },
    { id: "so2", name: "allergenNames.so2", icon: "/images/allergens/so2.svg" },
    { id: "soya", name: "allergenNames.soya", icon: "/images/allergens/soya.svg" },
    { id: "treenuts", name: "allergenNames.treenuts", icon: "/images/allergens/treenuts.svg" },
];

export const DIETARY_TAGS: DietaryTag[] = [
    {
        id: "vegan",
        name: "dietaryTagNames.vegan",
        emoji: "🍃",
        description: "dietaryTagDescriptions.vegan",
    },
    {
        id: "vegetarian",
        name: "dietaryTagNames.vegetarian",
        emoji: "🥕",
        description: "dietaryTagDescriptions.vegetarian",
    },
    {
        id: "gluten-free",
        name: "dietaryTagNames.gluten-free",
        emoji: "🌾",
        description: "dietaryTagDescriptions.gluten-free",
    },
    {
        id: "dairy-free",
        name: "dietaryTagNames.dairy-free",
        emoji: "🥛",
        description: "dietaryTagDescriptions.dairy-free",
    },
    {
        id: "nut-free",
        name: "dietaryTagNames.nut-free",
        emoji: "🥜",
        description: "dietaryTagDescriptions.nut-free",
    },
    {
        id: "mild",
        name: "dietaryTagNames.mild",
        emoji: "🌶️",
        description: "dietaryTagDescriptions.mild",
    },
    {
        id: "low-sodium",
        name: "dietaryTagNames.low-sodium",
        emoji: "🧂",
        description: "dietaryTagDescriptions.low-sodium",
    },
];
