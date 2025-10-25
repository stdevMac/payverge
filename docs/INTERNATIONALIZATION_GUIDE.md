# 🌍 Payverge Internationalization Guide

## Overview

Payverge uses a custom internationalization system built with React Context and JSON translation files. The system supports dynamic language switching and is designed for easy maintenance and scalability.

## Quick Start

### 1. Add Translation Keys

Add your translations to both language files:

**`/frontend/src/i18n/messages/en.json`**
```json
{
  "yourPageName": {
    "title": "Your Page Title",
    "subtitle": "Description text",
    "buttons": {
      "save": "Save",
      "cancel": "Cancel"
    },
    "errors": {
      "required": "This field is required"
    }
  }
}
```

**`/frontend/src/i18n/messages/es.json`**
```json
{
  "yourPageName": {
    "title": "Título de Tu Página",
    "subtitle": "Texto de descripción",
    "buttons": {
      "save": "Guardar",
      "cancel": "Cancelar"
    },
    "errors": {
      "required": "Este campo es obligatorio"
    }
  }
}
```

### 2. Set Up Component

```tsx
"use client";
import { useState, useEffect } from "react";
import { useSimpleLocale, getTranslation } from "@/i18n/SimpleTranslationProvider";

export default function YourPage() {
  const { locale } = useSimpleLocale();
  const [currentLocale, setCurrentLocale] = useState(locale);
  
  // Update translations when locale changes
  useEffect(() => {
    setCurrentLocale(locale);
  }, [locale]);
  
  // Translation helper
  const tString = (key: string): string => {
    const fullKey = `yourPageName.${key}`;
    const result = getTranslation(fullKey, currentLocale);
    return Array.isArray(result) ? result[0] || key : result as string;
  };

  return (
    <div>
      <h1>{tString('title')}</h1>
      <p>{tString('subtitle')}</p>
      <button>{tString('buttons.save')}</button>
    </div>
  );
}
```

## File Structure

```
frontend/src/i18n/
├── config.ts                    # Locale configuration (en, es)
├── SimpleTranslationProvider.tsx # Translation provider & context
└── messages/
    ├── en.json                  # English translations
    └── es.json                  # Spanish translations
```

## Key Naming Convention

### ✅ Good Structure
```json
{
  "pageName": {
    "title": "Main Title",
    "subtitle": "Description",
    "sections": {
      "header": "Header Text",
      "content": "Content Text"
    },
    "buttons": {
      "primary": "Primary Action",
      "secondary": "Secondary Action"
    },
    "errors": {
      "validation": "Validation Error",
      "network": "Network Error"
    },
    "messages": {
      "success": "Success Message",
      "loading": "Loading Message"
    }
  }
}
```

### ❌ Avoid
```json
{
  "page_title": "Bad naming",
  "btn1": "Unclear purpose",
  "error_msg_1": "Not descriptive"
}
```

## Advanced Usage

### Dynamic Content
```tsx
// JSON
{
  "welcome": "Welcome back, {name}!"
}

// Component
const message = tString('welcome').replace('{name}', user.name);
```

### Conditional Text
```tsx
// JSON
{
  "status": {
    "active": "Active",
    "inactive": "Inactive"
  }
}

// Component
const statusText = isActive 
  ? tString('status.active') 
  : tString('status.inactive');
```

### Arrays
```tsx
// JSON
{
  "options": ["Option 1", "Option 2", "Option 3"]
}

// Component
const options = getTranslation('pageName.options', currentLocale) as string[];
```

## Best Practices

### 1. Consistent Pattern
Always use this exact pattern for consistency:

```tsx
const { locale } = useSimpleLocale();
const [currentLocale, setCurrentLocale] = useState(locale);

useEffect(() => {
  setCurrentLocale(locale);
}, [locale]);

const tString = (key: string): string => {
  const fullKey = `yourPageName.${key}`;
  const result = getTranslation(fullKey, currentLocale);
  return Array.isArray(result) ? result[0] || key : result as string;
};
```

### 2. Key Organization
- Use **nested objects** for logical grouping
- Use **camelCase** for keys
- Group related translations together
- Keep keys descriptive and meaningful

### 3. Translation Quality
- Provide context for translators
- Keep strings concise but clear
- Consider cultural differences
- Test with longer translations (German, Spanish)

## Existing Pages

### Currently Internationalized
- **Landing Page** (`landing.*`)
- **Business Registration** (`businessRegister.*`)
- **Dashboard** (`dashboard.*`)

### Page Prefixes
- `landing` - Landing page content
- `businessRegister` - Business registration form
- `dashboard` - Main business dashboard

## Language Switching

The language switcher is available globally via `SimpleLanguageSwitcher` component. It's already included in the main layout, so individual pages don't need to include it.

### Current Languages
- **English** (`en`) - Default
- **Spanish** (`es`)

### Adding New Languages
1. Add locale to `/frontend/src/i18n/config.ts`
2. Create new JSON file in `/messages/` folder
3. Update `SimpleTranslationProvider.tsx` imports
4. Add language flag/name to config

## Troubleshooting

### Common Issues

**Translation keys showing instead of text:**
- Check JSON syntax and indentation
- Verify key exists in both language files
- Ensure correct page prefix in `tString` function

**Language switching not working:**
- Verify `useEffect` dependency array includes `[locale]`
- Check `currentLocale` state is being updated
- Ensure `SimpleTranslationProvider` wraps your app

**TypeScript errors:**
- Use type assertion `as string` if needed
- Check import paths are correct
- Verify JSON structure matches expected format

### Debug Tips
```tsx
// Add temporary logging
console.log('Current locale:', currentLocale);
console.log('Translation result:', getTranslation('yourPage.title', currentLocale));
```

## Checklist for New Pages

- [ ] Add translation keys to both `en.json` and `es.json`
- [ ] Import `useSimpleLocale` and `getTranslation`
- [ ] Set up locale state with `useState` and `useEffect`
- [ ] Create `tString` helper with correct page prefix
- [ ] Replace all hardcoded strings with `tString()` calls
- [ ] Test language switching works
- [ ] Verify text displays in both languages
- [ ] Check for text overflow with longer translations

## Examples

### Simple Page
```tsx
// pages/settings.tsx
const tString = (key: string): string => {
  const result = getTranslation(`settings.${key}`, currentLocale);
  return Array.isArray(result) ? result[0] || key : result as string;
};

return (
  <div>
    <h1>{tString('title')}</h1>
    <button>{tString('buttons.save')}</button>
  </div>
);
```

### Form with Validation
```tsx
// pages/profile.tsx
const tString = (key: string): string => {
  const result = getTranslation(`profile.${key}`, currentLocale);
  return Array.isArray(result) ? result[0] || key : result as string;
};

return (
  <form>
    <input 
      placeholder={tString('fields.name.placeholder')}
      error={errors.name ? tString('fields.name.error') : ''}
    />
    <button type="submit">
      {loading ? tString('buttons.saving') : tString('buttons.save')}
    </button>
  </form>
);
```

## Support

For questions or issues with internationalization:
1. Check this guide first
2. Look at existing implementations (dashboard, businessRegister)
3. Verify JSON syntax with online validators
4. Test with both languages before deployment

---

**Last Updated:** October 2025  
**Supported Languages:** English (en), Spanish (es)  
**Framework:** React + Next.js + Custom i18n System
