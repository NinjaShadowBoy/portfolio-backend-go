---
kind: publication
slug: cerc2026-benchmark
title: LinguoMT: A Zero-Shot Pilot Benchmark for African Language Speech Translation and Recognition
lang: en
url: /publications
pdf: /papers/linguomt-zero-shot-pilot-benchmark.pdf
venue: Collaborative European Research Conference (CERC 2026), ATU Galway, Ireland
venue_fr: Collaborative European Research Conference (CERC 2026), ATU Galway, Irlande
date: 2026-10
authors: Philippe Tamla, Ruth Stephie Darelle Bilogui Tiguena, Alex Nelson Ryan Abena, Gladys Kouakam, Haithem Afli
role: 3rd author
summary_en: A modular, reproducible benchmark of speech translation and speech recognition on Yoruba, Igbo, Hausa and Swahili, comparing SeamlessM4T-v2 Large with Whisper-large-v3 + NLLB. Validity checks, such as source-reference alignment and empty-output detection, ruled out invalid scores.
summary_fr: Un benchmark modulaire et reproductible de traduction et de reconnaissance vocales sur le yoruba, l'igbo, le haoussa et le swahili, comparant SeamlessM4T-v2 Large à Whisper-large-v3 + NLLB. Des contrôles de validité, comme l'alignement source-référence et la détection des sorties vides, ont permis d'écarter des scores invalides.
reviewed: false
---

# LinguoMT: A Zero-Shot Pilot Benchmark for African Language Speech Translation and Recognition

## Abstract

Recent advances in multilingual speech models have not benefited all languages equally. This paper presents the LinguoMT Zero-Shot Pilot Benchmark, a reproducible benchmark for evaluating multilingual speech translation and automatic speech recognition on low-resource African languages. It compares the end-to-end SeamlessM4T-v2 Large model and a cascaded Whisper-large-v3 plus NLLB-200-distilled-600M pipeline. Evaluations on Yoruba, Igbo, Hausa, and Swahili use the FLEURS validation split and the McGill-NLP African-Celtic corpus. The framework separates text- and audio-path evaluation to distinguish translation quality from speech-processing failures. Preliminary results show competitive BLEU and chrF2 scores from gold transcripts on African-Celtic, while Whisper-large-v3 produced empty outputs for all sampled Yoruba, Hausa, and Swahili audio items, suggesting routing or configuration issues rather than general model failure. FLEURS text-translation scores were withdrawn because source transcripts and English references were misaligned. Given the small pilot samples, results are interpreted as descriptive pipeline diagnostics rather than statistically representative comparisons.

Keywords: LinguoMT, speech translation, African languages, zero-shot evaluation

## Introduction

Recent advances in artificial intelligence (AI), multilingual natural language processing (NLP), and speech foundation models have expanded automatic speech recognition and speech translation capabilities. Systems such as Whisper (Radford et al., 2023), SeamlessM4T (Barrault et al., 2023), and NLLB-200 (NLLB Team, 2022) support multilingual speech and translation tasks across hundreds of languages.

However, low-resource African languages remain underrepresented in model development and systematic evaluation. Since speech is the primary communication modality for hundreds of millions of African language speakers, the absence of reliable speech translation technologies limits digital inclusion and reduces access to critical services in domains such as healthcare, education, public administration, and legal communication. Several scientific challenges arise from this gap. First, existing evaluations are fragmented across heterogeneous datasets, preprocessing strategies, experimental settings, and evaluation metrics, limiting reliable comparison. Second, the comparative behaviour of end-to-end architectures, such as SeamlessM4T-v2 and cascaded pipelines, such as Whisper-large-v3 combined with NLLB-200-distilled-600M, remains insufficiently understood for low-resource African languages under identical experimental conditions. Third, existing evaluations provide limited insight into whether degradation originates from automatic speech recognition, machine translation, or their interaction. To address these challenges, this research contributes to LinguoMT (Linguistic Intelligence for Multilingual Technologies), a broader research initiative for reproducible multilingual AI and language technologies for low-resource languages. Within this initiative, the present paper introduces the LinguoMT Zero-Shot Pilot Benchmark, which provides the common diagnostic benchmarking framework for multilingual speech translation and automatic speech recognition on selected African languages. Complementary studies extend this work by examining audio preprocessing in LinguoMT-Audio (Tamla et al., 2026b) and cascade versus end-to-end failure behaviour in tonal African languages in LinguoMT Evaluation (Tamla et al., 2026c). Accordingly, this study investigates two research questions. RQ1: How do end-to-end and cascaded multilingual speech translation architectures compare under zero-shot conditions across Yoruba, Igbo, Hausa, and Swahili, and where do their primary performance bottlenecks occur? RQ2: To what extent does degradation in the evaluated African-language conditions originate from acoustic processing, machine translation, or their interaction? The benchmark follows a lightweight adaptation of the systems-development methodology of Nunamaker et al. (1991) to structure problem identification, framework design, implementation, and pilot evaluation. Empirical validity is instead supported through dataset provenance, task-specific routing, controlled preprocessing, source-reference alignment, explicit sample accounting, standardized metrics, and reproducible experimental metadata. This paper makes four contributions: (1) a modular and reproducible benchmark for multilingual speech recognition and translation; (2) a task-level language-coverage audit distinguishing documented capabilities from executable routes; (3) paired textand audio-path diagnostics; and (4) provenance and validation mechanisms for detecting unsupported routes, empty outputs, and source-reference misalignment. The reported experiments are pilot-scale diagnostics intended to validate the proposed benchmark rather than provide statistically representative model comparisons.

## Related Work

Recent advances in multilingual AI and speech foundation models have improved speech recognition and translation, yet reproducible evaluation and architecture comparison remain limited for low-resource African languages.

### Multilingual Speech Foundation Models

Large multilingual speech foundation models have significantly advanced speech recognition and translation. Notable developments include SeamlessM4T, which offers a unified architecture for over 100 languages (Barrault et al., 2023), and Whisper, which utilizes weakly supervised training on approximately 680,000 hours of audio for competitive (Automatic Speech Recognition) ASR in 99 languages (Radford et al., 2023). NLLB-200 expanded multilingual neural machine translation to 200 languages, focusing on underrepresented ones (NLLB Team, 2022). MMS supports over 1,000 languages while performing well on multilingual benchmarks such as FLEURS (Pratap et al., 2024). However, existing evaluations frequently rely on heterogeneous datasets, preprocessing pipelines, and metrics, limiting reproducibility and preventing reliable architecture-level comparison under identical conditions. Remaining Challenge 1 (RC1): Existing multilingual speech translation research lacks a systematic and reproducible benchmark capable of comparing architectures on African languages under controlled zero-shot evaluation conditions.

### African Language Speech Resources and Evaluation

The development of African speech evaluation resources has accelerated, with AfriSpeech-200 introducing a pan-African benchmark for 120 accents across 33 countries, highlighting accent variation as a challenge for multilingual systems (Olatunji et al., 2023). XLS-R indicated that self-supervised multilingual speech representations perform well after fine-tuning on limited African language corpora but struggle with tonal and underrepresented languages (Babu et al., 2022). More recently, NaijaVoices introduced a large-scale Nigerian speech dataset covering diverse demographic and linguistic contexts (Emezue et al., 2025), while ÌroyìnSpeech addressed the preservation of tonal and diacritical properties in Yoruba speech (Ogunremi et al., 2024). However, these resources lack a shared zero-shot comparison for speech-translation systems. The LinguoMT benchmark aims to fill this gap through task-level coverage auditing, shared model adapters, provenance validation, and separate text-path and audio-path diagnostics. Remaining Challenge 2 (RC2): Existing benchmarks do not systematically quantify the performance disparity between African low-resource languages and high-resource multilingual reference languages under identical evaluation conditions.

### Cascade versus End-to-End Architectures

Multilingual speech translation research encompasses cascaded and end-to-end architectural paradigms. Cascaded systems separate ASR and machine translation, enabling intermediate errors to be inspected but introducing transcription-error propagation (Etchegoyhen et al., 2022). In contrast, end-to-end architectures like SeamlessM4T attempt to reduce this limitation by learning direct speech-to-translation mappings (Barrault et al., 2023); However, for low-resource African languages, controlled analysis of where degradation originates remains limited. Remaining Challenge 3 (RC3): Existing multilingual speech translation studies do not systematically isolate the relative contribution of acoustic modelling and translation quality to overall performance degradation for African languages.

The proposed framework must therefore support controlled comparison between audio-path and text-path performance under identical decoder conditions. Together, these challenges motivate the LinguoMT benchmark design presented in the next section. 

## Design and Methodology

This section presents the benchmarking framework designed to address RC1, RC2, and RC3 identified in the previous section. Framework Overview Figure 1 presents the principal interactions within the proposed benchmarking framework. The workflow begins with the loading of multilingual speech, text, and metadata datasets, followed by language selection and architecture configuration. The framework then executes multilingual inference tasks — speech-to-text translation, ASR, and machine translation — before computing evaluation metrics, comparing audio-path and text-path outputs, and identifying coverage gaps. This controlled workflow supports reproducible architecture comparison (RC1) and task-level coverage auditing across the evaluated languages and model routes (RC2).

Figure 1: Use-case diagram of the LinguoMT multilingual speech translation benchmarking framework

Figure 2 shows the LinguoMT component architecture, comprising input, preprocessing and routing, inference, evaluation, and analysis layers. The inference layer compares SeamlessM4T-v2 Large with the Whisper-large-v3 + NLLB-200-distilled-600M cascade, while the evaluation layer supports paired text- and audio-path analysis.

Figure 2: LinguoMT text-path and audio-path component architecture.

### Architecture Comparison

The evaluated architectures reveal complementary coverage constraints. SeamlessM4T-v2 Large supports Igbo speech processing but not Hausa speech translation, whereas the Whisper-large-v3 + NLLB-200-distilled-600M cascade supports Hausa translation but lacks a dedicated Igbo ASR token. Consequently, neither of the evaluated architectures provides complete task coverage across all four African languages simultaneously — a structural finding that motivates RC2.

### Audio-Path and Text-Path Separation

To address RC3, the framework uses two complementary paths. The text path feeds gold transcripts directly to the translation model, isolating translation from acoustic processing, while the audio path evaluates the full speech-processing pipeline. Comparing both paths helps locate candidate acoustic or translation bottlenecks, although causal attribution requires larger matched samples and controlled ablation.

### Debug-Mode Pilot Methodology

To support reproducible, low-cost benchmarking, the framework uses a debug mode with at most eight text and three audio samples per available language condition. Its purposes are to detect pipeline failures such as empty outputs or hallucinations, identify coverage gaps before larger compute allocations, and iteratively refine the evaluation tooling. Given the small sample sizes, results are treated as descriptive diagnostics only; no confidence intervals, significance tests, or model rankings are reported. The framework is designed to scale to full validation splits.

## Experimental Setup

This section describes the datasets, target languages, and evaluation metrics used for the controlled zero-shot pilot evaluation.

### Datasets

The evaluation uses two multilingual corpora with complementary conditions.

FLEURS (Conneau et al., 2022) provides multilingual speech recorded by native speakers at 16 kHz, with variability in speaking style and utterance structure; we use the validation split with configurations yo_ng (Yoruba), ha_ng (Hausa), ig_ng (Igbo), and sw_ke (Swahili). African-Celtic corpus (McGill-NLP, 2024) provides 48 kHz recordings with shorter, more controlled utterances; we use the development split. Table 2 summarises the pilot sample sizes. Table 2 summarises the debug-mode sample sizes. The pilot uses at most eight text and three audio samples per available language condition; African-Celtic Hausa provides eight text pairs but no audio. Results are therefore treated as descriptive diagnostics rather than statistically representative estimates.

Table 2: Dataset statistics — debug mode

| Corpus | Language | Text samples | Audio samples | Avg duration (s) | Sample rate (kHz) |
|---|---|---|---|---|---|
| African-Celtic | Igbo | 8 | 3 | 8.7 | 48 |
| African-Celtic | Yoruba | 8 | 3 | 12.5 | 48 |
| African-Celtic | Hausa | 8 | 0 | 10.1 | 48 |
| FLEURS | Igbo | 8 | 3 | 14.4 | 16 |
| FLEURS | Yoruba | 8 | 3 | 15.7 | 16 |
| FLEURS | Hausa | 8 | 3 | 17.3 | 16 |
| FLEURS | Swahili | 8 | 3 | 13.8 | 16 |

### Target Languages

The evaluation covers Yoruba and Igbo, two highly tonal Niger-Congo languages; Swahili, a non-tonal Bantu comparative control; and Hausa, an Afro-Asiatic Chadic language. Table 3 summarises their linguistic characteristics, model support, and corpus coverage. High-resource European reference languages are reserved for the full-scale study.

Table 3: Target language coverage. "Claimed" denotes documented language support; executable task routes were verified separately in the pilot

| Language | Family | Tonal | SeamlessM4T S2TT | Whisper ASR | African-Celtic | FLEURS |
|---|---|---|---|---|---|---|
| Igbo | Niger-Congo | Yes | Yes | No (no token) | Yes | Yes |
| Yoruba | Niger-Congo | Yes | Yes | Claimed; empty output in pilot | Yes | Yes |
| Hausa | Afro-Asiatic | No | No | Claimed; empty output in pilot | Text only (no audio) | Yes |
| Swahili | Niger-Congo | No | Yes | Claimed; empty output in pilot | No | Yes |

### Evaluation Metrics

The evaluation combines translation and ASR metrics to analyse both translation quality and acoustic robustness.

Translation quality is evaluated using corpus BLEU with 13a tokenisation (Papineni et al., 2002) and chrF2, the character n-gram F-score with beta=2 (Popovic, 2015), both computed via SacreBLEU (Post, 2018). chrF2 differs from chrF++, which additionally incorporates word n-grams; the full SacreBLEU metric signature is recorded with each run. ASR performance is evaluated using word error rate (WER) and character error rate (CER) via jiwer. These metrics support paired text- and audio-path analysis for bottleneck identification (RC3).

### Reproducibility Controls

Each run records model revisions, dataset splits, item identifiers, language-code mappings, task routes, preprocessing and decoding parameters, random seed, software versions, hardware, runtime, and metric signatures. Before inference and scoring, the pipeline verifies that source transcript, audio, and reference share the same stable identifier and flags unsupported routes or empty predictions. Recorded identifiers include the model revisions for facebook/seamless-m4t-v2-large, openai/whisper-large-v3, and facebook/nllb-200-distilled-600M.

## Results and Discussion

This section reports the pilot-scale evaluation of translation and ASR performance under zero-shot conditions, focusing on model coverage, acoustic robustness, and candidate architecture-level bottlenecks relevant to RQ1 and RQ2.

### Text-Path Translation

To address RC3, text-path experiments use gold transcripts to isolate translation performance from acoustic processing. Table 4 reports the African-Celtic results; FLEURS text-path scores are withdrawn because an artefact audit found that source transcripts and English references came from different records.

Table 4: African-Celtic text-path translation results (n=8 per condition; descriptive pilot)

| Model | Language | Direction | BLEU | chrF2 |
|---|---|---|---|---|
| SeamlessM4T-v2 Large | Igbo | Igbo to EN | 30.70 | 56.35 |
| SeamlessM4T-v2 Large | Igbo | EN to Igbo | 23.74 | 51.24 |
| SeamlessM4T-v2 Large | Yoruba | Yoruba to EN | 14.89 | 42.46 |
| SeamlessM4T-v2 Large | Yoruba | EN to Yoruba | 2.72 | 22.12 |
| Whisper-large-v3 + NLLB-200-distilled-600M | Yoruba | Yoruba to EN | 12.72 | 43.44 |
| Whisper-large-v3 + NLLB-200-distilled-600M | Yoruba | EN to Yoruba | 4.69 | 32.17 |
| Whisper-large-v3 + NLLB-200-distilled-600M | Hausa | Hausa to EN | 30.31 | 54.67 |
| Whisper-large-v3 + NLLB-200-distilled-600M | Hausa | EN to Hausa | 16.17 | 46.71 |

On African-Celtic, the highest observed source-to-English scores were BLEU 30.70/chrF2 56.35 for Igbo with SeamlessM4T-v2 Large and BLEU 30.31/chrF2 54.67 for Hausa with the Whisper-large-v3 + NLLB-200-distilled-600M cascade (n=8). Reverse-direction performance was lower, particularly for English-to-Yoruba, where SeamlessM4T-v2 Large achieved BLEU 2.72/chrF2 22.12 and showed over-generation (prediction/reference ratio 1.59). The cascaded architecture shows a smaller effect (BLEU 4.69; ratio 1.14). Given the pilot-scale sample, these differences across models, languages, and directions are descriptive and do not establish statistically supported model rankings. Direction asymmetry is observed in all evaluated conditions, with source-to-English outperforming English-to-source; the largest difference is 27.98 BLEU for Igbo under SeamlessM4T-v2 Large. Because the FLEURS text-path pairs were misaligned, no corpus-level text comparison is drawn; a corrected matched-record rerun is required. FLEURS audio-path results remain unaffected and are retained as pilot diagnostics.

### ASR Evaluation

The ASR experiments identify acoustic processing and task routing as the candidate bottlenecks in this pilot, addressing RQ1 at pilot scale only. Whisper-large-v3 returned empty outputs for all sampled Yoruba, Hausa, and Swahili items in the tested configuration, indicating an unsupported or misconfigured route, or another decoding failure requiring investigation; this does not establish general model failure. Igbo was not evaluated because no dedicated Whisper language token was available, and African-Celtic Hausa contained no sampled audio.

Table 5: ASR results — audio path, pilot mode (n=3 per condition).

| Model | Language / corpus | WER | CER | Notes |
|---|---|---|---|---|
| SeamlessM4T-v2 Large | Igbo / African-Celtic | 1.014 | 0.524 | Word insertions; 0 empty outputs; n=3 |
| SeamlessM4T-v2 Large | Yoruba / African-Celtic | 1.011 | 0.410 | Word insertions; 0 empty outputs; n=3 |
| SeamlessM4T-v2 Large | Igbo / FLEURS | 0.797 | 0.369 | 0 empty outputs; n=3 |
| SeamlessM4T-v2 Large | Yoruba / FLEURS | 0.863 | 0.342 | 0 empty outputs; n=3 |
| SeamlessM4T-v2 Large | Swahili / FLEURS | 0.474 | 0.198 | Lowest observed error rates; 0 empty outputs; n=3 |
| Whisper-large-v3 | Yoruba / African-Celtic | 1.000 | 1.000 | 3 of 3 outputs empty; n=3 |
| Whisper-large-v3 | Yoruba / FLEURS | 1.000 | 1.000 | 3 of 3 outputs empty; n=3 |
| Whisper-large-v3 | Hausa / FLEURS | 1.000 | 1.000 | 3 of 3 outputs empty; n=3 |
| Whisper-large-v3 | Swahili / FLEURS | 1.000 | 1.000 | 3 of 3 outputs empty; n=3 |

SeamlessM4T-v2 Large produced non-empty outputs in all evaluated conditions. On African-Celtic, WER exceeded 1.0 for Igbo and Yoruba because of word insertions, while CER remained below 0.53, indicating partial character-level overlap. Swahili showed the lowest observed FLEURS error rates (WER 0.474, CER 0.198). These pilot results do not isolate tonal effects and may reflect model coverage, routing, normalization, acoustic conditions, data mismatch, or decoding configuration. Larger matched samples are required for tone-aware analysis.

### Answering the Research Questions

RQ1 — Both architectures produced meaningful text-path results where coverage existed, but only SeamlessM4T-v2 Large generated non-empty audio-path outputs. Speech processing and task routing therefore emerge as candidate bottlenecks in this pilot, without supporting general model rankings. RQ2 — Paired text- and audio-path analysis suggests that degradation in the sampled conditions is primarily associated with the audio path rather than translation decoding. No corpus-level text comparison is drawn because the FLEURS text pairs were misaligned.

## Threats to Validity

The study is limited by its pilot-scale sample, uneven audio availability, task-specific model coverage, and absence of inferential analysis. FLEURS text-path results were withdrawn because of source-reference misalignment, Whisper Igbo was unavailable in the configured interface, and African-Celtic Hausa contained no sampled audio. The remaining observations validate pipeline behaviour and expose failure modes; they do not establish model rankings, language difficulty, tonal effects, or population-level performance. Generalisability is further limited by differences between the two corpora and by the selected languages and recording conditions. BLEU and chrF2 may not capture tonal or pragmatic correctness, WER may disadvantage some orthographic structures, and the prediction/reference length ratio is only a proxy for hallucination. The English-to-Yoruba over-generation finding is supported within eight sampled pairs, and Whisper returned empty outputs in all 12 sampled items across four conditions. All other findings remain preliminary and require larger-scale validation.

## Conclusion

This paper presented the LinguoMT Zero-Shot Pilot Benchmark, a reproducible framework combining task-level coverage auditing with paired text- and audio-path diagnostics for low-resource African speech translation and ASR. The pilot produced meaningful African-Celtic text-path outputs while exposing audio-path routing and decoding failures. Whisper-large-v3 returned empty outputs in all sampled configured conditions, while FLEURS text-path scores were withdrawn because of source-reference misalignment. Given the small samples, the findings are descriptive diagnostics rather than model- or language-level conclusions. Future work will rerun identifier-matched FLEURS experiments, scale evaluation, extend language coverage, and incorporate high-resource references, and broader African speech resources.
