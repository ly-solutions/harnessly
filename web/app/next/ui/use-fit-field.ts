// A text field as tall as its text, as the chat's field is: it grows line by line up to the max height its CSS gives,
// then the text scrolls inside. The height is set on the element from its scrollHeight whenever the text changes.
import { useLayoutEffect, type RefObject } from 'react';

const fit = (el: HTMLTextAreaElement) => {
  el.style.height = 'auto';
  el.style.height = `${el.scrollHeight}px`;
};

export const useFitField = (field: RefObject<HTMLTextAreaElement | null>, text: string) => {
  useLayoutEffect(() => {
    if (field.current) fit(field.current);
  }, [field, text]);
};
