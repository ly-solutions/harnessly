declare module '@novnc/novnc' {
  export default class RFB {
    constructor(target: HTMLElement, url: string);
    scaleViewport: boolean;
    viewOnly: boolean;
    focusOnClick: boolean;
    addEventListener(type: 'connect' | 'disconnect', listener: () => void): void;
    disconnect(): void;
  }
}
