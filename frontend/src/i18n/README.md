# Payverge Internationalization System

A comprehensive internationalization (i18n) system for the Payverge platform supporting multiple languages with easy extensibility.

## 🌍 Supported Languages

- **English (en)** - Default language
- **Spanish (es)** - Full translation support

## 📁 File Structure

```
src/i18n/
├── README.md                 # This documentation
├── config.ts                 # Core configuration and types
├── utils.ts                  # Utility functions for i18n
├── TranslationProvider.tsx   # React context provider
├── messages/
│   ├── en.json              # English translations
│   └── es.json              # Spanish translations
```

## 🚀 Quick Start

### 1. Using Translations in Components

```tsx
import { useTranslations } from '@/i18n/TranslationProvider';

function MyComponent() {
  const { t } = useTranslations('landing'); // Use 'landing' namespace
  
  return (
    <div>
      <h1>{t('hero.title')}</h1>
      <p>{t('hero.subtitle')}</p>
    </div>
  );
}
```

### 2. Using Translations with Parameters

```tsx
const { t } = useTranslations('common');

// Translation with parameters
const message = t('welcome', { name: 'John' });
// If translation is "Welcome, {{name}}!" -> "Welcome, John!"
```

### 3. Getting Current Locale

```tsx
import { useLocale } from '@/i18n/TranslationProvider';

function MyComponent() {
  const locale = useLocale(); // 'en' or 'es'
  
  return <div>Current language: {locale}</div>;
}
```

### 4. Changing Language

```tsx
import { useTranslations } from '@/i18n/TranslationProvider';

function LanguageSwitcher() {
  const { setLocale } = useTranslations();
  
  return (
    <button onClick={() => setLocale('es')}>
      Switch to Spanish
    </button>
  );
}
```

## 📝 Translation File Structure

Translation files use nested JSON structure for organization:

```json
{
  "landing": {
    "hero": {
      "badge": "Coming soon",
      "title": "From the table to your wallet,",
      "subtitle": "Payverge makes dining payments borderless...",
      "cta": {
        "register": "Register Business",
        "dashboard": "Access Dashboard"
      }
    }
  },
  "common": {
    "loading": "Loading...",
    "error": "Something went wrong"
  }
}
```

## 🔧 Configuration

### Adding New Languages

1. **Add locale to config:**
```typescript
// src/i18n/config.ts
export const locales = ['en', 'es', 'fr'] as const; // Add 'fr'

export const languageNames = {
  en: 'English',
  es: 'Español',
  fr: 'Français' // Add French
} as const;

export const languageFlags = {
  en: '🇺🇸',
  es: '🇪🇸', 
  fr: '🇫🇷' // Add French flag
} as const;
```

2. **Create translation file:**
```bash
# Create new translation file
cp src/i18n/messages/en.json src/i18n/messages/fr.json
# Edit fr.json with French translations
```

3. **Update TranslationProvider:**
```typescript
// src/i18n/TranslationProvider.tsx
import frMessages from './messages/fr.json';

const messages = {
  en: enMessages,
  es: esMessages,
  fr: frMessages // Add French messages
};
```

## 🎯 Best Practices

### 1. Namespace Organization
- Use clear namespaces: `landing`, `dashboard`, `common`, etc.
- Group related translations together
- Keep common translations in the `common` namespace

### 2. Key Naming
- Use descriptive, hierarchical keys: `hero.title`, `form.validation.required`
- Use camelCase for consistency
- Avoid overly deep nesting (max 3-4 levels)

### 3. Parameter Usage
- Use `{{paramName}}` syntax for parameters
- Keep parameter names descriptive
- Validate parameters exist before using

### 4. Fallback Strategy
- English is the fallback language
- Missing keys return the key itself
- Console warnings for missing translations in development

## 🔄 Migration from Legacy System

The new system replaces the old custom translation context:

### Before (Legacy):
```tsx
import { useTranslation } from '@/i18n/useTranslation';

const { t } = useTranslation();
const text = t('some.key');
```

### After (New System):
```tsx
import { useTranslations } from '@/i18n/TranslationProvider';

const { t } = useTranslations('namespace');
const text = t('key');
```

## 🛠️ Development Tools

### Adding New Translations
1. Add keys to `en.json` first (source of truth)
2. Add corresponding translations to other language files
3. Use the translation in components
4. Test with language switcher

### Debugging
- Missing translation keys are logged to console
- Keys return themselves if translation not found
- Check browser localStorage for `locale` key

## 🚀 Future Enhancements

### Planned Features
- **URL-based routing** (`/es/dashboard`)
- **Server-side rendering** with next-intl integration
- **Automatic translation** via Google Translate API
- **Pluralization support** for complex grammar rules
- **Date/number formatting** per locale
- **RTL language support** for Arabic, Hebrew

### Performance Optimizations
- **Code splitting** by language
- **Lazy loading** of translation files
- **Translation caching** for better performance

## 📊 Current Implementation Status

✅ **Completed:**
- Basic translation system with React Context
- English and Spanish support
- Landing page fully translated
- Language switcher component
- localStorage persistence
- Fallback system

🔄 **In Progress:**
- Extending translations to all pages
- Business dashboard translations

📋 **Planned:**
- URL-based routing
- Server-side rendering
- Additional languages (French, German, Portuguese)

## 🤝 Contributing

When adding new features that need translations:

1. Add English text to appropriate namespace in `en.json`
2. Add Spanish translation to `es.json`
3. Use the translation in your component
4. Test with both languages
5. Update this README if adding new patterns

## 📞 Support

For questions about the internationalization system:
- Check this README first
- Look at existing implementations in the codebase
- Test with the language switcher component
- Ensure translations are properly namespaced

---

**Note:** This system is designed to be extensible and maintainable. Follow the established patterns when adding new translations or languages.
