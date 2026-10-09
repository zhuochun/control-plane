import ELK, {
  type ElkNode,
  type ElkLayoutArguments,
} from "elkjs/lib/elk-api.js";
import workerUrl from "elkjs/lib/elk-worker.min.js?url";

// Mermaid uses only layout(). Give each call an isolated worker and release it
// after success or failure; repeated report visits must not accumulate workers.
export default class ElkLayout {
  async layout(graph: ElkNode, options?: ElkLayoutArguments): Promise<ElkNode> {
    const worker = new Worker(workerUrl);
    const failed = new Promise<never>((_, reject) => {
      const onFailure = () =>
        reject(new Error("Diagram layout worker failed."));
      worker.addEventListener("error", onFailure, { once: true });
      worker.addEventListener("messageerror", onFailure, { once: true });
    });
    try {
      const elk = new ELK({ workerFactory: () => worker });
      // Mermaid attaches renderer callbacks (such as node.intersect) to its
      // graph. ELK consumes JSON geometry; those functions stay in Mermaid's
      // original node map and must not cross the worker boundary.
      const geometry: ElkNode = JSON.parse(
        JSON.stringify(graph, (_, value) =>
          typeof value === "function" ? undefined : value,
        ),
      );
      return await Promise.race([elk.layout(geometry, options), failed]);
    } finally {
      worker.terminate();
    }
  }
}
