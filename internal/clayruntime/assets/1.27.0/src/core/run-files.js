import { saveToken } from "./host-attrs.js";

const RUN_FILE = /^\/_\/wire\/runs\/[A-Za-z0-9_-]+\/files\/[A-Za-z0-9_-]+$/;

const coded = (code, message) => Object.assign(new Error(message), { code });

// A wire helper's run files live 5 minutes after the run, so a caller starts every
// fetch as soon as the run ends and hands out that one promise. The host finds the
// document from its address and checks the save token; without both it answers 404.
// `what` names the file in messages ("source", "output"); every failure is `code`.
export async function fetchRunFile(url, code, what) {
  if (typeof url !== "string" || !RUN_FILE.test(url)) {
    throw coded(code, `The host named no file for this ${what}.`);
  }
  const headers = { "Document-URL": window.location.href };
  const token = saveToken();
  if (typeof token === "string" && token) headers["Save-Token"] = token;
  let response;
  try {
    response = await fetch(new URL(url, window.location.origin).href, { headers, cache: "no-store", redirect: "manual" });
  } catch {
    throw coded(code, `This ${what} could not be fetched from the host.`);
  }
  if (!response.ok) throw coded(code, `The host answered ${response.status} for this ${what}. Run the request again.`);
  try {
    return await response.text();
  } catch {
    throw coded(code, `This ${what} could not be read from the host. Run the request again.`);
  }
}
