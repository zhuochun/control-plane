export type ReportBlock = {
  id: string;
  type:
    | "markdown"
    | "image"
    | "diagram"
    | "review"
    | "choice"
    | "text_input"
    | "actions";
  body_md?: string;
  title?: string;
  blocks?: ReportBlock[];
  format?: { id: string; version: number };
  submit_label?: string;
  question?: string;
  required?: boolean;
  selection?: "single" | "multiple";
  min_selections?: number;
  max_selections?: number;
  options?: { id: string; label: string; blocks?: ReportBlock[] }[];
  language?: "mermaid" | "plantuml";
  source?: string;
  artifact_id?: string;
  caption?: string;
  description?: string;
  action_ids?: string[];
};

export type AnswerValue = {
  field_id: string;
  disposition: "answered" | "skipped";
  selected_ids?: string[];
  text?: string;
};

export type ReviewAnswer = {
  id: string;
  review_id: string;
  title: string;
  content_version: number;
  review_material_hash: string;
  submitted_at: string;
  supersedes?: string;
  note: string;
  applicable: boolean;
  values: (AnswerValue & { question: string; type: string; answer: string })[];
};
