declare module 'react-signature-canvas' {
  import * as React from 'react';

  interface SignatureCanvasProps {
    penColor?: string;
    canvasProps?: React.CanvasHTMLAttributes<HTMLCanvasElement>;
    ref?: React.RefObject<SignatureCanvas>;
    [key: string]: any;
  }

  class SignatureCanvas extends React.Component<SignatureCanvasProps> {
    isEmpty(): boolean;
    clear(): void;
    toDataURL(type?: string, encoderOptions?: any): string;
  }

  export default SignatureCanvas;
}
