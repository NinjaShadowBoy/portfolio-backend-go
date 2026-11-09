---
kind: publication
slug: cerc2026-tonal
title: LinguoMT: Cascade versus End-to-End Speech Translation for Tonal African Languages
lang: en
url: /publications
pdf: /papers/linguomt-cascade-vs-end-to-end-tonal-languages.pdf
venue: Collaborative European Research Conference (CERC 2026), ATU Galway, Ireland
venue_fr: Collaborative European Research Conference (CERC 2026), ATU Galway, Irlande
date: 2026-10
authors: Philippe Tamla, Alex Nelson Ryan Abena, Ruth Stephie Darelle Bilogui Tiguena, Gladys Kouakam, Haithem Afli
presented: true
role: 2nd author
summary_en: Compares a cascade (Whisper-large-v3 + NLLB) with an end-to-end model (SeamlessM4T-v2) for speech translation of tonal Niger-Congo languages, Igbo and Yoruba, in zero-shot conditions. I defined a six-category error taxonomy and annotated 30 Igbo samples by hand to separate hallucination, semantic drift and speech recognition failures.
summary_fr: Compare une cascade (Whisper-large-v3 + NLLB) et un modèle de bout en bout (SeamlessM4T-v2) pour la traduction vocale de langues nigéro-congolaises tonales, l'igbo et le yoruba, en zero-shot. J'ai défini une taxonomie d'erreurs à six catégories et annoté 30 échantillons igbo à la main pour distinguer hallucination, dérive sémantique et échecs de reconnaissance vocale.
reviewed: false
---

# LinguoMT: Cascade versus End-to-End Speech Translation for Tonal African Languages

## Abstract

Multilingual speech translation systems frequently cite tonal African languages as supported, yet their failure modes under zero-shot deployment remain poorly characterised. This paper presents LinguoMT Evaluation, a framework for comparing end-to-end and cascaded speech translation for Igbo and Yoruba, two tonal Niger-Congo languages. Using the Few-shot Learning Evaluation of Universal Representations of Speech (FLEURS) and the African-Celtic corpus, the evaluation isolates three dimensions: automatic speech recognition quality, text-path translation ceiling, and corpus domain effects. Results show an approximately 70-fold difference in text-path BLEU between the two evaluated corpora for Igbo: SeamlessM4T-v2 Large achieves BLEU 30.70 on African-Celtic and 0.43 on FLEURS. A six-category manual error taxonomy applied to 30 Igbo samples shows that direct end-to-end translation is dominated by hallucination and semantic drift, while cascade output — when the automatic speech recognition stage succeeds — is substantially more reliable. Swahili, used as a non-tonal control, achieved a WER of 0.47, compared with 0.80–0.86 for Igbo and Yoruba, suggesting within this limited pilot sample that tonal acoustic encoding may contribute to the observed performance gap.

Keywords: speech translation, tonal languages, cascade architecture, African languages

## Introduction

Multilingual speech translation has advanced substantially through large-scale foundation models such as SeamlessM4T-v2 Large (Barrault et al., 2023) and the cascaded Whisper-large-v3 plus No Language Left Behind (NLLB)-distilled-600M pipeline (Radford et al., 2023; NLLB Team, 2022).

These architectures report coverage across hundreds of languages, yet aggregate multilingual capability often conceals severe degradation for underrepresented low-resource languages. This is particularly acute for tonal Niger-Congo languages such as Igbo and Yoruba, where lexical meaning depends on pitch variation and tone realisation. Multilingual acoustic encoders trained predominantly on non-tonal high-resource corpora may fail to preserve tone-sensitive lexical information during cross-modal inference, leading to hallucinated or semantically drifted outputs. The debate between cascade-based and end-to-end speech translation architectures (Etchegoyhen et al., 2022) has not been resolved for low-resource African tonal languages. Cascade systems offer modularity and diagnostic transparency — automatic speech recognition (ASR) errors and machine translation (MT) errors can be inspected separately — but error propagation from the ASR stage constrains overall quality. End-to-end architectures such as SeamlessM4T eliminate the intermediate transcription stage but place greater demands on the acoustic encoder to establish stable cross-modal alignment. Recent benchmarks such as NaijaS2ST (Maltais et al., 2026) report that both paradigms remain limited for Nigerian languages under zero-shot conditions, yet the specific failure mechanisms responsible for degradation have not been systematically characterised through qualitative error analysis. Three problem statements motivate this study. PS1: Direct end-to-end speech translation for tonal African languages is highly unstable under zero-shot conditions, yet the failure modes — whether hallucination, semantic drift, or acoustic encoding failure — have not been systematically identified. PS2: No qualitative error taxonomy has been established for cascade versus end-to-end speech translation on tonal African languages, preventing practitioners from diagnosing whether degradation originates from the ASR stage, the MT stage, or the acoustic-to-semantic alignment layer. PS3: The impact of corpus domain on text-path translation ceiling performance for African languages has not been quantified, making it impossible to separate genuine model limitations from corpus-specific effects in evaluation benchmarks. The study addresses two research questions. RQ1: How do direct end-to-end and cascade-based speech translation architectures compare in tonal versus non-tonal African languages under zero-shot conditions, and where is the primary bottleneck located in the translation pipeline? RQ2: What error categories characterise translation failure in end-to-end and cascade speech translation for tonal languages, and how does corpus domain affect the text-path translation ceiling independently of architecture? The research follows the systems development methodology of Nunamaker et al. (1991), which interleaves Theory Building, System Development, and Observation and Experimentation in a mutually reinforcing cycle. Theory Building establishes the comparative framework and error taxonomy. System Development operationalizes three evaluation paths — direct end-to-end, text-mediated oracle, and cascade — under controlled zero-shot conditions. Observation and Experimentation analyses quantitative performance across corpora and languages and applies the manual error taxonomy to diagnose qualitative failure modes. Section 2 reviews related work and identifies remaining challenges. Section 3 presents the system design. Section 4 reports results and discussion. Section 5 concludes. This paper is a specialised contribution within the broader LinguoMT research initiative.

The LinguoMT Zero-Shot Pilot Benchmark establishes a reproducible benchmarking framework for evaluating multilingual speech translation and automatic speech recognition on low-resource African languages (Tamla et al., 2026a). LinguoMT-Audio complements this work by evaluating four preprocessing strategies applied to large-scale architectures using the African-Celtic corpus (Tamla et al., 2026b). The present paper’s distinct contribution is the error taxonomy, tonal-language analysis, and corpus-domain gap quantification.

## Related Work

Multilingual speech processing has expanded substantially through community-driven African NLP and speech resource initiatives. The Masakhane project (Adelani et al., 2021) demonstrated that collaborative data creation substantially improves natural language processing (NLP) performance for African languages. In the speech domain, datasets such as IroyinSpeech (Ogunremi et al., 2024), AfriSpeech-200 (Olatunji et al., 2023), and NaijaVoices (Emezue et al., 2025) have expanded the availability of speech data for tonal African languages. Despite this progress, controlled cascade versus end-to-end speech translation evaluation for tonal African languages remains limited, and qualitative failure analysis is largely absent from the existing literature. The first remaining challenge (RC1) is the lack of controlled comparative evaluation between cascade and end-to-end speech translation architectures specifically for tonal African languages. Cascade architectures combine ASR with MT, providing modularity but introducing error propagation (Etchegoyhen et al., 2022). End-to-end architectures such as SeamlessM4T attempt direct speech-to-translation mapping within a single model using a wav2vec 2.0-derived encoder (Baevski et al., 2020). Recent multilingual benchmarks such as NaijaS2ST (Maltais et al., 2026) report near-zero performance for Hausa and highlight persistent coverage limitations in multilingual speech encoders for Nigerian languages. However, these studies do not systematically compare failure behaviour across architectures for tonal versus non-tonal African languages under identical zero-shot conditions, leaving practitioners without guidance on which paradigm is more appropriate for tonal language deployment. The second remaining challenge (RC2) is the absence of a qualitative error taxonomy capable of systematically identifying tone-related translation failures in multilingual speech translation. Bilingual Evaluation Understudy (Papineni et al., 2002) and character F-score (ChrF; Popovic, 2015) remain standard translation quality metrics but provide no insight into the type of failure — whether hallucination, semantic drift, repetitive looping, or partial keyword recovery. Belinkov and Glass (2019) argue that qualitative error analysis is essential for understanding neural language system behaviour, particularly in low-resource settings where automatic metrics may assign non-trivial scores to fluent but semantically grounded outputs, and low scores to partially correct tonal translations. Neural metrics such as Crosslingual Optimized Metric for Evaluation of Translation (COMET) may improve sensitivity but can still assign high scores to hallucinated output in low-resource conditions (Li et al., 2025). The third remaining challenge (RC3) concerns the undercharacterised impact of corpus domain on text-path translation ceiling performance for African languages.

FLEURS (Conneau et al., 2022) provides parallel speech across 102 languages and is widely used for multilingual evaluation, but the source text style, vocabulary, and sentence structure differ substantially from field-collected corpora such as the African-Celtic corpus. When text-path evaluation on FLEURS yields near-zero BLEU for languages that achieve 30 BLEU on African-Celtic under identical model and decoder conditions, the discrepancy cannot be attributed to model capability alone. Understanding whether this domain gap reflects vocabulary mismatch, reference translation style, or sentence-level complexity is essential for correctly interpreting benchmark performance and for making fair comparisons across evaluation settings.

## Design

Figure 1 presents the component architecture of the proposed evaluation framework, designed to address RC1 by enabling controlled comparison across three translation paths under identical conditions. The architecture consists of five layers: input, preprocessing, inference, evaluation, and analysis. The input layer contains FLEURS and African-Celtic speech and text data, including audio recordings, reference transcripts, target translations, and language metadata. The preprocessing layer resamples all audio to 16 kHz under identical conditions. The inference layer implements three evaluation paths. The direct end-to-end path passes raw audio through the SeamlessM4T-v2 Large acoustic encoder and multilingual decoder. The text-mediated oracle path provides gold transcripts directly to the text encoder, bypassing acoustic processing entirely and establishing the translation quality ceiling. The cascade path uses Whisper-large-v3 for ASR followed by NLLB-distilled-600M for translation. This three-path design isolates whether failure originates from acoustic encoding, ASR error propagation, or the translation decoder itself. The evaluation layer computes BLEU and ChrF for translation quality and WER and character error rate (CER) for acoustic transcription. The analysis layer applies the six-category error taxonomy addressing RC2, and quantifies corpus domain effects addressing RC3.

Figure 1: Component architecture of the proposed cascade versus end-to-end evaluation framework.

### Error Taxonomy

To address RC2, the study introduces a six-category manual error taxonomy for analysing qualitative failure behaviour in multilingual speech translation for tonal languages. Thirty Igbo samples are annotated across the three translation paths. The taxonomy covers: Satisfactory output (accurate translation with acceptable meaning preservation); Paraphrase (correct meaning expressed with different wording, acceptable quality); Keyword Spotting (partial entity or lexical recognition without full semantic coverage); Semantic Drift (output diverges progressively from source meaning through plausible but incorrect continuation); Hallucination (fluent, grammatical output that is semantically unrelated to the source audio or transcript); and Repetitive Looping (decoder trapped in a repetition pattern producing the same phrase or sequence multiple times). Hallucination and semantic drift are classified as severe failures because they produce misleading output without signalling uncertainty. The taxonomy is applied to complement BLEU-based evaluation, since aggregate metrics cannot distinguish between these qualitatively different failure modes.

Table 1: Six-category error taxonomy for tonal language speech translation failure analysis.

| Category | Description | Severity |
|---|---|---|
| Satisfactory | Accurate translation with preserved meaning | Good |
| Paraphrase | Correct meaning, different wording | Acceptable |
| Keyword Spotting | Partial entity/lexical recognition only | Partial |
| Semantic Drift | Output progressively diverges from source meaning | Severe |
| Hallucination | Fluent but semantically unrelated to source | Severe |
| Repetitive Looping | Decoder trapped in repeated phrase generation | Severe |

## Experimental Protocol

All experiments are conducted under zero-shot conditions with no task-specific fine-tuning or language adaptation. SeamlessM4T-v2 Large is configured for multilingual speech-to-text translation with beam-search decoding (beam size 5). The cascade path uses Whisper-large-v3 for ASR followed by NLLB-distilled-600M for English translation. Both systems are evaluated on identical data under identical decoding conditions to ensure that performance differences reflect architectural properties rather than evaluation artefacts. The tonal languages evaluated are Igbo and Yoruba (Niger-Congo, tonal); the non-tonal control is Swahili (Bantu, non-tonal). Hausa is an Afro-Asiatic tone language and is included in selected Whisper+NLLB experiments on the African-Celtic corpus. Debug-mode evaluation uses 8 text samples and 3 audio samples per language, providing directional pilot results consistent with the exploratory scope of this study.

## Results and Discussion

### Corpus Domain Gap

Table 2 reports text-path BLEU scores across both corpora, directly addressing RQ2 (corpus domain effects, RC3). The results reveal a striking domain gap: SeamlessM4T-v2 Large achieves 30.70 BLEU for Igbo to English and 14.89 BLEU for Yoruba to English on African-Celtic, yet scores collapse to 0.43 and 0.59 respectively on FLEURS under identical text-path conditions using the same model and decoder. This represents a 70-fold difference for Igbo. The pattern holds for Swahili (0.41 BLEU on FLEURS versus African-Celtic absence from the SeamlessM4T experiment set) and across both architectures: Whisper+NLLB achieves 30.31 BLEU for Hausa to English on African-Celtic but only 0.46 on FLEURS. Because the text path bypasses acoustic processing, the observed difference is unlikely to originate from ASR errors. However, the present pilot does not isolate its precise cause. Corpus-specific factors such as vocabulary, sentence complexity, reference-translation style, text normalization, and sample selection may have contributed to the observed gap. This finding has important implications for multilingual speech translation evaluation: benchmark selection substantially changes the apparent capability of a model, independent of the architecture itself.

Table 2: Text-path BLEU across corpora (gold transcript input, zero-shot). The domain gap between African-Celtic and FLEURS is up to 70x for Igbo.

| Architecture | Language | Direction | African-Celtic BLEU | FLEURS BLEU |
|---|---|---|---|---|
| SeamlessM4T | Igbo | Igbo→EN | 30.70 | 0.43 |
| SeamlessM4T | Igbo | EN→Igbo | 23.74 | 0.43 |
| SeamlessM4T | Yoruba | Yoruba→EN | 14.89 | 0.59 |
| SeamlessM4T | Yoruba | EN→Yoruba | 2.72 | 0.31 |
| SeamlessM4T | Swahili | Swahili→EN | — | 0.41 |
| Whisper+NLLB | Yoruba | Yoruba→EN | 12.72 | 0.57 |
| Whisper+NLLB | Hausa | Hausa→EN | 30.31 | 0.46 |
| Whisper+NLLB | Swahili | Swahili→EN | — | 0.45 |

### Tonal Language ASR Bottleneck

Table 3 reports ASR results across languages and architectures, addressing RQ1 (RC1). Swahili, the non-tonal control language, achieves WER 0.47 and CER 0.20 with SeamlessM4T — substantially better than the tonal languages. Igbo achieves WER 0.80 (CER 0.37) and Yoruba WER 0.86 (CER 0.34), representing a 40–45% relative increase in word-level error rate compared to Swahili. This pattern suggests that tonal acoustic characteristics may contribute to the observed transcription difficulties, although corpus composition, recording conditions, sample selection, and language coverage in the training data may also have influenced the results.

Since SeamlessM4T uses a wav2vec 2.0-derived acoustic encoder (Baevski et al., 2020), one possible explanation is that the learned speech representations may not capture tone-sensitive acoustic cues equally well across languages. Whisper-large-v3 produces entirely empty ASR outputs for all tested African languages (WER=1.0, num_empty=3/3), confirming the cascade failure mode identified in the companion audio preprocessing study: without proper sampling-rate handling and adequate tonal language representation in training data, the ASR stage silently collapses.

Table 3: ASR acoustic stability (SeamlessM4T-v2 Large). Swahili (non-tonal control) substantially outperforms tonal languages.

| Architecture | Language | Tonal | WER | CER |
|---|---|---|---|---|
| SeamlessM4T | Swahili | No | 0.47 | 0.20 |
| SeamlessM4T | Yoruba | Yes | 0.86 | 0.34 |
| SeamlessM4T | Igbo | Yes | 0.80 | 0.37 |
| Whisper-v3 | Yoruba | Yes | 1.00 | 1.00 |
| Whisper-v3 | Hausa | Yes | 1.00 | 1.00 |
| Whisper-v3 | Swahili | No | 1.00 | 1.00 |

### Error Taxonomy Analysis

Table 4 reports the manual error distribution across the three translation paths for Igbo, directly addressing RQ1 (RC2). The text-mediated oracle path produces substantially more satisfactory outputs (29 satisfactory, 53 paraphrase) than the direct end-to-end path (16 satisfactory, 42 paraphrase). This pattern suggests that the translation decoder may preserve semantic content more effectively when provided with an accurate textual representation than when translation is performed directly from the audio input. In contrast, the direct end-to-end path is dominated by severe failure: 40 hallucination instances and 46 semantic drift instances out of 150 total annotations. This distribution is consistent with challenges in acoustic-to-semantic alignment within the SeamlessM4T direct end-to-end path for the evaluated Igbo samples. These challenges may contribute to the generation of fluent but semantically ungrounded English outputs. The cascade path produces even higher hallucination counts (60 instances) than direct end-to-end, reflecting total ASR failure when Whisper produces empty transcriptions that are passed as empty inputs to NLLB, triggering decoder hallucination. The keyword spotting category (5 direct E2E, 14 cascade) suggests partial acoustic grounding: the decoder recovers proper nouns or content words but fails to produce a coherent full translation.

Table 4: Manual error taxonomy distribution across translation paths (30 Igbo samples, 5 annotations each = 150 total).

| Error category | Direct E2E | Cascade | Text oracle |
|---|---|---|---|
| Satisfactory | 16 | 2 | 29 |
| Paraphrase | 42 | 22 | 53 |
| Keyword Spotting | 5 | 14 | 4 |
| Semantic Drift | 46 | 50 | 50 |
| Hallucination | 40 | 60 | 12 |
| Repetitive Looping | 1 | 2 | 2 |

### Translation Direction Asymmetry

A notable finding across both architectures is the severe asymmetry between source-to-English and English-to-source translation performance. On African-Celtic, SeamlessM4T achieves 30.70 BLEU for Igbo to English but only 2.72 BLEU for English to Yoruba — a ratio of approximately 1.59 output words per reference word, indicating significant hallucination in the reverse direction. Similarly, Whisper+NLLB achieves 30.31 BLEU for Hausa to English but 16.17 for English to Hausa, and only 4.69 for English to Yoruba. This asymmetry reflects a systematic bias in both architectures toward English as the target language, consistent with the dominance of English in multilingual training distributions. For African language communities, the English-to-source direction is often more practically relevant — for healthcare communication, educational content translation, or humanitarian information dissemination — yet current systems perform substantially worse in this direction. Addressing this asymmetry requires not only more African speech data but also balanced bidirectional training objectives in multilingual model design.

## Discussion

The results suggest three interrelated findings. First, the African-Celtic/FLEURS text-path difference is the largest observed performance variation in the present pilot evaluation. The observed BLEU difference between African-Celtic and FLEURS for Igbo text-path translation is approximately 70-fold; however, the present evaluation does not establish which corpus-specific factors are responsible for this difference. Because both evaluations used the same text-path configuration, this difference is unlikely to originate from ASR errors or acoustic input processing. It may instead reflect corpus-specific factors, including vocabulary, sentence complexity, reference-translation style, text normalization, or sample selection. Second, Swahili achieved lower WER (0.47) than tonal languages (0.80–0.86), suggesting that tonal acoustic characteristics may contribute to transcription challenges. Third, the error taxonomy reveals that cascade failure produces more hallucinations than direct end-to-end failure when ASR silently collapses, while direct end-to-end failure is more evenly distributed across hallucination and semantic drift.

For tonal African language deployment, cascade architectures are therefore safer when ASR quality can be guaranteed, but their silent failure mode under poor acoustic conditions creates substantial risk in heterogeneous field deployments.

## Threats to Validity

Regarding internal validity, the error taxonomy annotations are performed on debug-mode samples (30 Igbo sentences), limiting statistical generalisability. The taxonomy categories are defined a priori without inter-annotator agreement measurement, introducing potential subjectivity in boundary cases between semantic drift and hallucination. Regarding external validity, findings may not generalise beyond Igbo and Yoruba to other tonal language families such as Bantu tone languages or Khoisan click languages, which differ substantially in tonal complexity and acoustic properties. The WER advantage of Swahili over tonal languages may also partially reflect corpus-level factors such as speaking style and recording quality rather than tonality alone. Regarding construct validity, BLEU remains a surface metric that is particularly unreliable at very low scores (below 1.0), and the near-zero FLEURS results should be interpreted as indicative of severe mismatch rather than precise performance estimates. The corpus domain gap interpretation assumes that model and decoder conditions are held constant across corpora, which is satisfied by the text-path design but would require further validation under audio-path conditions.

## Conclusion

This paper presented a comparative evaluation of cascade-based and end-to-end speech translation architectures for tonal African languages within the Nunamaker systems development framework. The study addressed three remaining challenges: insufficient comparative evaluation of cascade versus end-to-end architectures for tonal languages (RC1), the absence of a qualitative error taxonomy for tonal language speech translation failures (RC2), and the undercharacterised impact of corpus domain on translation ceiling performance (RC3). Within the evaluated sample, Swahili achieved a lower WER of 0.47 than Igbo and Yoruba, which obtained WER values of 0.80 and 0.86, respectively. This pattern suggests that tonal acoustic characteristics may contribute to the observed transcription challenges. The substantial BLEU difference between African-Celtic and FLEURS for Igbo text-path translation further suggests that benchmark and corpus characteristics may substantially influence observed model performance. The six-category error taxonomy identified more hallucination annotations in the cascade path than in the direct end-to-end path, while the latter showed errors distributed across hallucination and semantic drift. Collectively, these findings indicate that multilingual speech translation coverage claims for tonal African languages require corpus-stratified evaluation and qualitative error analysis before practitioners can reliably assess system readiness for deployment. Future work will investigate tone-aware acoustic pre-training strategies, adaptive multilingual speech encoders, and larger tonal African speech corpora. Expanding the error taxonomy to include inter-annotator reliability measurement and applying it to Yoruba, Hausa, and additional African tonal languages will further validate the qualitative findings.

Corpus domain analysis using sentence-level complexity and vocabulary overlap metrics will help disentangle model limitations from benchmark-specific characteristics in future evaluations.
