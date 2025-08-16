declare module "multipart-parser" {
    export interface FilePart {
        filename: string;
        data: Uint8Array;
        type: string;
    }

    export interface FieldPart {
        name: string;
        data: string;
    }

    export function Parse(
        data: Uint8Array,
        boundary: string,
    ): (FilePart | FieldPart)[];
}
