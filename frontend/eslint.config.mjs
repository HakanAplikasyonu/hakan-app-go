import { defineConfig, globalIgnores } from "eslint/config";
import nextVitals from "eslint-config-next/core-web-vitals";
import nextTs from "eslint-config-next/typescript";
import eslintConfigPrettier from "eslint-config-prettier";

const eslintConfig = defineConfig([
  ...nextVitals,
  ...nextTs,
  eslintConfigPrettier,
  {
    files: ["src/**/*.{ts,tsx}"],
    ignores: ["src/components/ui/date-picker.tsx"],
    rules: {
      "@typescript-eslint/no-unused-vars": [
        "warn",
        {
          argsIgnorePattern: "^_",
          varsIgnorePattern: "^_",
          caughtErrorsIgnorePattern: "^_",
          ignoreRestSiblings: true,
        },
      ],
      "no-restricted-syntax": [
        "error",
        {
          selector: "JSXAttribute[name.name='type'][value.value='date']",
          message:
            'Use DatePicker from @/components/ui/date-picker (or AppDatePicker in forms) instead of native type="date".',
        },
      ],
    },
  },
  // Native öğeler yerine @/components/ui (#520). İstisnalar ve gerekçeleri:
  // docs/arastirma/native-bilesen-denetimi.md ("Bilinçli istisnalar").
  // Aynı kurala ikinci girdi öncekini ezer: type="date" seçicisi burada da var.
  {
    files: ["src/**/*.tsx"],
    ignores: [
      "src/components/ui/**",
      "src/components/editor/**",
      "src/app/global-error.tsx",
      "src/features/legal/components/legal-markdown.tsx",
      "src/components/tables/editable-cell.tsx",
      "src/components/realtime/realtime-debug-panel.tsx",
      "src/**/*.test.tsx",
    ],
    rules: {
      "no-restricted-syntax": [
        "error",
        {
          selector: "JSXAttribute[name.name='type'][value.value='date']",
          message:
            'Use DatePicker from @/components/ui/date-picker (or AppDatePicker in forms) instead of native type="date".',
        },
        {
          selector:
            "JSXOpeningElement[name.name=/^(button|select|textarea|table|label|details|summary|dialog|fieldset|progress|meter)$/]",
          message:
            "Native öğe yerine @/components/ui karşılığını kullanın (Button, NativeSelect/Select, Textarea, Table, Label, Collapsible, Dialog, FieldSet, Progress). Bkz. docs/arastirma/native-bilesen-denetimi.md.",
        },
        {
          // Gizli dosya girişleri istisna: <input type="file" className="sr-only|hidden">.
          selector:
            "JSXOpeningElement[name.name='input']:not(:has(JSXAttribute[name.name='type'][value.value='file']))",
          message:
            "Native <input> yerine Input, InputGroup, Checkbox, RadioGroup, DatePicker ya da MonthPicker kullanın.",
        },
        {
          selector:
            "JSXOpeningElement[name.name='Button'] > JSXAttribute[name.name='title']",
          message:
            "Düğmede title= yerine Hint (@/components/ui/hint) ya da Tooltip kullanın; dokunmatikte ve klavyede title görünmez.",
        },
        {
          selector:
            "CallExpression[callee.object.name='window'][callee.property.name=/^(alert|confirm|prompt)$/]",
          message:
            "Tarayıcı diyaloğu yerine useDialogs() ya da toast kullanın.",
        },
      ],
      "no-restricted-globals": [
        "error",
        { name: "alert", message: "toast kullanın." },
        { name: "confirm", message: "useDialogs().confirm kullanın." },
        { name: "prompt", message: "useDialogs().prompt kullanın." },
      ],
    },
  },
  // Editor X (Lexical / shadcn-editor) uses ref-during-render and effect
  // patterns that conflict with React Compiler eslint rules.
  {
    files: ["src/components/editor/**/*.{ts,tsx}"],
    rules: {
      "react-hooks/refs": "off",
      "react-hooks/set-state-in-effect": "off",
      "react-hooks/use-memo": "off",
      "react/display-name": "off",
      "@next/next/no-img-element": "off",
      "@typescript-eslint/no-empty-object-type": "off",
      "@typescript-eslint/ban-ts-comment": "off",
    },
  },
  globalIgnores([
    ".next/**",
    "out/**",
    "build/**",
    "next-env.d.ts",
    "src/generated/**",
    "src/features/help/content.generated.ts",
  ]),
]);

export default eslintConfig;
