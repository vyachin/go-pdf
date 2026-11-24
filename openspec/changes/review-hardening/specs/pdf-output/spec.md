# Spec Delta

## ADDED Requirements

### Requirement: Font objects form a complete reference chain

Every embedded font SHALL be written as five distinct objects: Type0, CIDFontType2, FontDescriptor, the FontFile2 stream and the ToUnicode stream. `/DescendantFonts` MUST reference the CIDFontType2 object, its `/FontDescriptor` entry MUST reference the descriptor, and the descriptor MUST reference the font file stream. No object identifier may be written twice for different object types.

#### Scenario: Descendant font reference
- **WHEN** a PDF is produced from a template containing text
- **THEN** following `/DescendantFonts` from the Type0 font leads to an object with `/Subtype /CIDFontType2`
- **AND** that object is not a `/FontDescriptor`

#### Scenario: Descriptor and font file
- **WHEN** the CIDFontType2 object is followed through `/FontDescriptor`
- **THEN** the descriptor exists
- **AND** its `/FontFile2` entry references a stream object, not another dictionary

#### Scenario: Text remains extractable
- **WHEN** a document with Cyrillic text is produced after the fix
- **THEN** text extraction returns the original characters

#### Scenario: Independent validation
- **WHEN** the generated PDF is checked by an independent validator that resolves object references
- **THEN** no structural error is reported
